package uscis

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

const (
	loginPageURL = "https://myaccount.uscis.gov/sign-in"
)

// Login performs authentication using headless browser with manual 2FA support
// Returns the session cookie string in format: name=value
// If 2FA is required, prompts user to enter verification code via stdin
func Login(username, password string) (string, error) {
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
	var screenshotBuf []byte

	// Perform login and wait for AWS WAF challenges
	err := chromedp.Run(ctx,
		chromedp.Navigate(loginPageURL),
		chromedp.WaitVisible(`#email-address`, chromedp.ByQuery),
		chromedp.SendKeys(`#email-address`, username, chromedp.ByQuery),
		chromedp.SendKeys(`#password`, password, chromedp.ByQuery),
		chromedp.WaitEnabled("sign-in-btn", chromedp.ByID),
		chromedp.Click("sign-in-btn", chromedp.ByID),
		chromedp.Sleep(10*time.Second), // Wait for AWS WAF challenges and redirects
		chromedp.FullScreenshot(&screenshotBuf, 90),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := os.WriteFile(fmt.Sprintf("after_signin_%s.png", time.Now().Format("2006-01-02T15-04-05")), screenshotBuf, 0644); err != nil {
				log.Printf("Failed to save screenshot: %v\n", err)
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
				return err
			}
			log.Printf("Current URL after login: %s\n", currentURL)
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			if err != nil {
				log.Printf("Failed to get cookies after signin: %v", err)
			}
			log.Printf("Check cookies after signin\n")
			for _, cookie := range cookies {
				log.Printf("\tcookie: %s=%s\n", cookie.Name, cookie.Value)
			}
			return err
		}),
	)
	if err != nil {
		return "", fmt.Errorf("login automation failed: %w", err)
	}

	// Handle 2FA if required
	if strings.Contains(currentURL, "/auth") {
		log.Println("2FA verification required - please check your email for the verification code")

		fmt.Print("Enter 2FA verification code: ")
		reader := bufio.NewReader(os.Stdin)
		code, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read verification code: %w", err)
		}
		code = strings.TrimSpace(code)

		log.Println("Submitting verification code...")
		err = chromedp.Run(ctx,
			// use SendKeys
			// when using JavaScript to set value directly, after submitting, the input field gets cleared and error "Secure verification code cannot be blank
			chromedp.WaitEnabled(`secure-verification-code`, chromedp.ByID),
			chromedp.SendKeys(`#secure-verification-code`, code, chromedp.ByQuery),
			chromedp.FullScreenshot(&screenshotBuf, 90), // Screenshot BEFORE clicking
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := os.WriteFile(fmt.Sprintf("before_2fa_submit_%s.png", time.Now().Format("2006-01-02T15-04-05")), screenshotBuf, 0644); err != nil {
					log.Printf("Failed to save pre-submit screenshot: %v\n", err)
				}
				return nil
			}),
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
			chromedp.Sleep(10*time.Second),              // Wait for verification
			chromedp.FullScreenshot(&screenshotBuf, 90), // Screenshot AFTER clicking
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := os.WriteFile(fmt.Sprintf("after_2fa_submit_%s.png", time.Now().Format("2006-01-02T15-04-05")), screenshotBuf, 0644); err != nil {
					log.Printf("Failed to save post-submit screenshot: %v\n", err)
				}
				return nil
			}),
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
					return err
				}
				log.Printf("Current URL after auth: %s\n", currentURL)
				return nil
			}),
			chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				cookies, err = network.GetCookies().Do(ctx)
				if err != nil {
					log.Printf("Failed to get cookies after 2fa: %v\n", err)
				}
				log.Printf("Check cookies after 2fa\n")
				for _, cookie := range cookies {
					log.Printf("\tcookie: %s=%s\n", cookie.Name, cookie.Value)
				}
				return nil
			}),
		)

		if err != nil {
			return "", fmt.Errorf("2FA submission failed: %w", err)
		}

		log.Println("2FA verification completed successfully")
	}

	// Extract session cookie
	cookieNames := []string{"_uscis_user_session", "_myuscis_session_rx"}
	for _, cookieName := range cookieNames {
		for _, cookie := range cookies {
			if cookie.Name == cookieName {
				return fmt.Sprintf("%s=%s", cookie.Name, cookie.Value), nil
			}
		}
	}

	return "", fmt.Errorf("required session cookie not found after login")
}
