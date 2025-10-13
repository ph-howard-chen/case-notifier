package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// go build -o test_login test_login.go
// ./test_login

const (
	loginPageURL = "https://myaccount.uscis.gov/sign-in"
	applicantURL = "https://my.uscis.gov/account/applicant"
)

func main_test() {
	// Get credentials from environment
	username := os.Getenv("USCIS_USERNAME")
	password := os.Getenv("USCIS_PASSWORD")

	if username == "" || password == "" {
		log.Fatal("Please set USCIS_USERNAME and USCIS_PASSWORD environment variables")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Configure headless browser with bot detection evasion
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36`),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	log.Println("Starting login automation...")
	var cookies []*network.Cookie
	var currentURL string

	// Perform login and wait for AWS WAF challenges
	err := chromedp.Run(ctx,
		chromedp.Navigate(loginPageURL),
		chromedp.WaitVisible(`#email-address`, chromedp.ByQuery),
		chromedp.SendKeys(`#email-address`, username, chromedp.ByQuery),
		chromedp.SendKeys(`#password`, password, chromedp.ByQuery),
		chromedp.WaitEnabled("sign-in-btn", chromedp.ByID),
		chromedp.Click("sign-in-btn", chromedp.ByID),
		chromedp.Sleep(10*time.Second), // Wait for AWS WAF challenges and redirects
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
				return err
			}
			log.Printf("Current URL after login: %s", currentURL)
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)
	if err != nil {
		log.Fatalf("login automation failed: %v", err)
	}

	// Handle 2FA if required
	if strings.Contains(currentURL, "/auth") {
		log.Println("2FA verification required - please check your email for the verification code")

		fmt.Print("Enter 2FA verification code: ")
		reader := bufio.NewReader(os.Stdin)
		code, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("failed to read verification code: %v", err)
		}
		code = strings.TrimSpace(code)

		log.Println("Submitting verification code...")
		err = chromedp.Run(ctx,
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
			chromedp.Sleep(10*time.Second), // Wait for verification
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				cookies, err = network.GetCookies().Do(ctx)
				return err
			}),
		)

		if err != nil {
			log.Fatalf("2FA submission failed: %v", err)
		}

		log.Println("2FA verification completed successfully")
	}

	// Navigate to applicant page to initialize session for API access
	err = chromedp.Run(ctx,
		chromedp.Navigate(applicantURL),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to load applicant page: %v", err)
	} else {
		log.Println("Applicant page loaded successfully")
	}

	// Now try API access
	log.Println("Testing API access using browser session...")
	testCaseID := "IOE0933798378"
	var apiResponse string
	err = chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("https://my.uscis.gov/account/case-service/api/cases/%s", testCaseID)),
		chromedp.Sleep(2*time.Second), // Wait for API response
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Extract the JSON from the <pre> tag
			return chromedp.Text("pre", &apiResponse, chromedp.ByQuery).Do(ctx)
		}),
	)

	if err != nil {
		log.Fatalf("Failed to access API: %v", err)
	}

	log.Println("\n=== API ACCESS SUCCESSFUL ===")
	log.Printf("API response:\n%s\n", apiResponse)

	// Extract and display cookie
	log.Println("\n=== COOKIES ===")
	cookieNames := []string{"_uscis_user_session", "_myuscis_session_rx"}
	for _, cookieName := range cookieNames {
		for _, cookie := range cookies {
			if cookie.Name == cookieName {
				log.Printf("%s=%s", cookie.Name, cookie.Value)
			}
		}
	}
}
