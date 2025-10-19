package uscis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// EmailFetcher is an interface for fetching 2FA codes from email
type EmailFetcher interface {
	FetchLatest2FACode(senderEmail string, maxWaitTime time.Duration) (string, error)
}

const (
	loginPageURL = "https://myaccount.uscis.gov/sign-in"
	applicantURL = "https://my.uscis.gov/account/applicant"
	caseAPIURL   = "https://my.uscis.gov/account/case-service/api/cases"
)

// BrowserClient uses chromedp browser automation for authentication and API access
// The browser session is kept alive and used for all API calls
type BrowserClient struct {
	ctx             context.Context
	cancel          context.CancelFunc
	allocCancel     context.CancelFunc
	uscisUsername   string
	uscisPassword   string
	emailClient     EmailFetcher  // Optional: for automated 2FA
	email2FASender  string        // Sender email for 2FA emails
	email2FATimeout time.Duration // Timeout for waiting for 2FA email
}

// NewBrowserClient creates a new browser client and performs login with 2FA support
// The browser session remains active and is used for subsequent API calls
// Call Close() when done to cleanup resources
func NewBrowserClient(uscisUsername, uscisPassword string) (*BrowserClient, error) {
	return NewBrowserClientWithEmail(uscisUsername, uscisPassword, nil, "", 5*time.Minute)
}

// NewBrowserClientWithEmail creates a new browser client with automated email 2FA support
// If emailClient is nil, falls back to manual stdin prompt for 2FA
func NewBrowserClientWithEmail(uscisUsername, uscisPassword string, emailClient EmailFetcher, email2FASender string, email2FATimeout time.Duration) (*BrowserClient, error) {
	log.Printf("Creating browser client...")

	// Create context without timeout - we want to keep it alive
	ctx := context.Background()

	// Configure headless browser with bot detection evasion
	log.Printf("Configuring Chrome options...")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36`),
	)

	log.Printf("Creating Chrome allocator context...")
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)

	log.Printf("Creating browser context...")
	browserCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))

	client := &BrowserClient{
		ctx:             browserCtx,
		cancel:          cancel,
		allocCancel:     allocCancel,
		uscisUsername:   uscisUsername,
		uscisPassword:   uscisPassword,
		emailClient:     emailClient,
		email2FASender:  email2FASender,
		email2FATimeout: email2FATimeout,
	}

	// Perform login
	if err := client.login(); err != nil {
		client.Close()
		// Wrap login failure in ErrAuthenticationFailed for consistent error handling
		return nil, &ErrAuthenticationFailed{StatusCode: 0} // 0 indicates browser login failure (not HTTP status)
	}

	return client, nil
}

// login performs the authentication flow with 2FA support
func (bc *BrowserClient) login() error {
	log.Printf("Starting login automation...")
	log.Printf("Username: %s", bc.uscisUsername)
	log.Printf("Password: %s (length: %d)", strings.Repeat("*", len(bc.uscisPassword)), len(bc.uscisPassword))
	var currentURL string

	// Perform login and wait for AWS WAF challenges
	log.Printf("Navigating to login page: %s", loginPageURL)
	err := chromedp.Run(bc.ctx,
		chromedp.Navigate(loginPageURL),
		chromedp.WaitVisible(`#email-address`, chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to load login page: %w", err)
	}

	log.Printf("Entering credentials...")
	err = chromedp.Run(bc.ctx,
		chromedp.SendKeys(`#email-address`, bc.uscisUsername, chromedp.ByQuery),
		chromedp.SendKeys(`#password`, bc.uscisPassword, chromedp.ByQuery),
		chromedp.WaitEnabled("sign-in-btn", chromedp.ByID),
	)
	if err != nil {
		return fmt.Errorf("failed to enter credentials: %w", err)
	}

	log.Printf("Clicking sign-in button...")
	err = chromedp.Run(bc.ctx,
		chromedp.Click("sign-in-btn", chromedp.ByID),
	)
	if err != nil {
		return fmt.Errorf("failed to click sign-in button: %w", err)
	}

	log.Printf("Waiting for redirect after sign-in (AWS WAF challenges may take time)...")
	// Poll for URL change with timeout
	maxWait := 60 * time.Second
	checkInterval := 2 * time.Second
	startTime := time.Now()

	for {
		elapsed := time.Since(startTime)
		if elapsed > maxWait {
			return fmt.Errorf("timeout waiting for redirect after sign-in (still on %s after %v)", currentURL, elapsed)
		}

		err = chromedp.Run(bc.ctx,
			chromedp.Sleep(checkInterval),
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
					return err
				}
				log.Printf("Current URL: %s (elapsed: %.0fs)", currentURL, elapsed.Seconds())
				return nil
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to check URL: %w", err)
		}

		// Check if we've been redirected away from sign-in page
		if !strings.Contains(currentURL, "/sign-in") {
			log.Printf("Redirected away from sign-in page to: %s", currentURL)
			break
		}
	}

	// Handle 2FA if required
	if strings.Contains(currentURL, "/auth") {
		log.Printf("2FA required - URL contains /auth")
		if err := bc.handle2FA(); err != nil {
			return err
		}
		log.Printf("2FA verification completed successfully")
	} else {
		log.Printf("No 2FA required - already redirected to: %s", currentURL)
	}

	// Navigate to applicant page to initialize session for API access
	log.Printf("Navigating to applicant page %s to finalize login", applicantURL)
	err = chromedp.Run(bc.ctx,
		chromedp.Navigate(applicantURL),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to load applicant page: %w", err)
	}

	log.Printf("Login completed successfully, browser session ready for API calls")
	return nil
}

// handle2FA handles the 2FA flow by fetching code from email or prompting user
func (bc *BrowserClient) handle2FA() error {
	log.Printf("2FA verification required")

	var code string
	var err error

	// Try automated email fetch if configured
	if bc.emailClient != nil && bc.email2FASender != "" {
		log.Printf("Attempting automated 2FA code fetch from email...")
		log.Printf("  Email sender: %s", bc.email2FASender)
		log.Printf("  Timeout: %v", bc.email2FATimeout)
		log.Printf("Waiting for 2FA email (this may take up to %v)...", bc.email2FATimeout)

		code, err = bc.emailClient.FetchLatest2FACode(bc.email2FASender, bc.email2FATimeout)
		if err != nil {
			log.Printf("Failed to fetch 2FA code from email: %v", err)
			log.Printf("Falling back to manual input...")
		} else {
			log.Printf("Successfully retrieved 2FA code from email")
		}
	} else {
		log.Printf("Automated email fetch not configured")
	}

	// Fall back to manual input if email fetch failed or not configured
	if code == "" {
		log.Printf("Please check your email for the verification code")
		fmt.Print("Enter 2FA verification code: ")
		reader := bufio.NewReader(os.Stdin)
		code, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read verification code: %w", err)
		}
		code = strings.TrimSpace(code)
	}

	log.Printf("Submitting verification code...")
	var currentURL string
	err = chromedp.Run(bc.ctx,
		// use SendKeys - JavaScript value setting gets cleared on submit
		chromedp.WaitEnabled(`secure-verification-code`, chromedp.ByID),
		chromedp.SendKeys(`#secure-verification-code`, code, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use JavaScript to click button since chromedp.Click doesn't work reliably here
			var exists bool
			if err := chromedp.Evaluate(`document.getElementById('2fa-submit-btn') !== null`, &exists).Do(ctx); err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("submit button not found in DOM")
			}
			return chromedp.Evaluate(`document.getElementById('2fa-submit-btn').click()`, nil).Do(ctx)
		}),
		chromedp.Sleep(5*time.Second), // Wait for verification
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
				return err
			}
			log.Printf("Current URL after 2FA: %s\n", currentURL)
			return nil
		}),
	)

	if err != nil {
		return fmt.Errorf("2FA submission failed: %w", err)
	}

	return nil
}

// RefreshSession re-authenticates by running the login flow again
// Useful when the browser session expires during long-running polling
func (bc *BrowserClient) RefreshSession() error {
	log.Printf("Refreshing browser session...")
	return bc.login()
}

// FetchCaseStatus fetches case status by navigating to the API URL in the browser
// Automatically retries once with session refresh if the response indicates auth failure
func (bc *BrowserClient) FetchCaseStatus(caseID string) (map[string]interface{}, error) {
	result, err := bc.fetchCaseStatusInternal(caseID)

	// Check if response indicates authentication failure
	shouldRefresh := false
	if result != nil {
		if data, ok := result["data"]; ok && data == nil {
			// API returned null data, might be auth issue
			shouldRefresh = true
		}
	}

	// If we detect possible auth failure, try to refresh and retry once
	if shouldRefresh {
		log.Printf("Possible session expiration detected (null data), attempting to refresh...")

		if refreshErr := bc.RefreshSession(); refreshErr != nil {
			log.Printf("Failed to refresh session: %v", refreshErr)
			// Return ErrAuthenticationFailed for consistent error handling
			return nil, &ErrAuthenticationFailed{StatusCode: 0} // 0 indicates session refresh failure
		}

		log.Printf("Session refreshed, retrying request...")
		result, err = bc.fetchCaseStatusInternal(caseID)
	}

	return result, err
}

// fetchCaseStatusInternal performs the actual API call via browser navigation
func (bc *BrowserClient) fetchCaseStatusInternal(caseID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s", caseAPIURL, caseID)
	log.Printf("Navigating to API URL: %s", url)

	var apiResponse string
	err := chromedp.Run(bc.ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(2*time.Second), // Wait for API response
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Extract the JSON from the <pre> tag
			return chromedp.Text("pre", &apiResponse, chromedp.ByQuery).Do(ctx)
		}),
	)

	if err != nil {
		log.Printf("Failed to navigate to API URL: %v", err)
		return nil, fmt.Errorf("failed to navigate to API URL: %w", err)
	}

	log.Printf("API response received (length: %d bytes)", len(apiResponse))
	if len(apiResponse) > 200 {
		log.Printf("API response preview: %s...", apiResponse[:200])
	} else {
		log.Printf("API response: %s", apiResponse)
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(apiResponse), &result); err != nil {
		log.Printf("Failed to parse API response as JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if data field is null
	if data, ok := result["data"]; ok {
		if data == nil {
			log.Printf("API returned null data - possible session issue")
		} else {
			log.Printf("API returned valid data")
		}
	}

	return result, nil
}

// Close cleans up the browser resources
func (bc *BrowserClient) Close() error {
	if bc.cancel != nil {
		bc.cancel()
	}
	if bc.allocCancel != nil {
		bc.allocCancel()
	}
	return nil
}
