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
	username        string
	password        string
	emailClient     EmailFetcher  // Optional: for automated 2FA
	email2FASender  string        // Sender email for 2FA emails
	email2FATimeout time.Duration // Timeout for waiting for 2FA email
}

// NewBrowserClient creates a new browser client and performs login with 2FA support
// The browser session remains active and is used for subsequent API calls
// Call Close() when done to cleanup resources
func NewBrowserClient(username, password string) (*BrowserClient, error) {
	return NewBrowserClientWithEmail(username, password, nil, "", 5*time.Minute)
}

// NewBrowserClientWithEmail creates a new browser client with automated email 2FA support
// If emailClient is nil, falls back to manual stdin prompt for 2FA
func NewBrowserClientWithEmail(username, password string, emailClient EmailFetcher, email2FASender string, email2FATimeout time.Duration) (*BrowserClient, error) {
	// Create context without timeout - we want to keep it alive
	ctx := context.Background()

	// Configure headless browser with bot detection evasion
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36`),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	browserCtx, cancel := chromedp.NewContext(allocCtx)

	client := &BrowserClient{
		ctx:             browserCtx,
		cancel:          cancel,
		allocCancel:     allocCancel,
		username:        username,
		password:        password,
		emailClient:     emailClient,
		email2FASender:  email2FASender,
		email2FATimeout: email2FATimeout,
	}

	// Perform login
	if err := client.login(); err != nil {
		client.Close()
		return nil, err
	}

	return client, nil
}

// login performs the authentication flow with 2FA support
func (bc *BrowserClient) login() error {
	log.Printf("Starting login automation...")
	var currentURL string

	// Perform login and wait for AWS WAF challenges
	err := chromedp.Run(bc.ctx,
		chromedp.Navigate(loginPageURL),
		chromedp.WaitVisible(`#email-address`, chromedp.ByQuery),
		chromedp.SendKeys(`#email-address`, bc.username, chromedp.ByQuery),
		chromedp.SendKeys(`#password`, bc.password, chromedp.ByQuery),
		chromedp.WaitEnabled("sign-in-btn", chromedp.ByID),
		chromedp.Click("sign-in-btn", chromedp.ByID),
		chromedp.Sleep(10*time.Second), // Wait for AWS WAF challenges and redirects
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
				return err
			}
			log.Printf("Current URL after login: %s\n", currentURL)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("login automation failed: %w", err)
	}

	// Handle 2FA if required
	if strings.Contains(currentURL, "/auth") {
		if err := bc.handle2FA(); err != nil {
			return err
		}
	}

	// Navigate to applicant page to initialize session for API access
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
		log.Printf("Fetching 2FA code from email (sender: %s)...", bc.email2FASender)
		code, err = bc.emailClient.FetchLatest2FACode(bc.email2FASender, bc.email2FATimeout)
		if err != nil {
			log.Printf("Failed to fetch 2FA code from email: %v", err)
			log.Printf("Falling back to manual input...")
		}
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

	log.Printf("Submitting verification code %s...", code)
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

	log.Printf("2FA verification completed successfully")
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
			return nil, fmt.Errorf("session refresh failed: %w (original error: %v)", refreshErr, err)
		}

		log.Printf("Session refreshed, retrying request...")
		result, err = bc.fetchCaseStatusInternal(caseID)
	}

	return result, err
}

// fetchCaseStatusInternal performs the actual API call via browser navigation
func (bc *BrowserClient) fetchCaseStatusInternal(caseID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s", caseAPIURL, caseID)

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
		return nil, fmt.Errorf("failed to navigate to API URL: %w", err)
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(apiResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
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
