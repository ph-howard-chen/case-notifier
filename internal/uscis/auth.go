package uscis

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	loginPageURL = "https://myaccount.uscis.gov/sign-in"
)

// Login performs basic authentication using headless browser (no 2FA support yet)
// Returns the session cookie string in format: name=value
func Login(username, password string) (string, error) {
	// 1. Set up a longer timeout for potentially slow network/JS challenges
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 2. Add flags to make the headless browser harder to detect
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true), // Crucial for running in a container
		chromedp.Flag("disable-dev-shm-usage", true),
		// --- Key changes for bot detection evasion ---
		chromedp.Flag("disable-blink-features", "AutomationControlled"),                                                                       // Removes the "navigator.webdriver" flag
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36`), // Use a common user agent
	)

	// Create allocator context
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	// Create chromedp context
	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	fmt.Print("start chromedp.Run()\n")
	// This slice will hold the cookies after a successful login
	var cookies []*network.Cookie

	var screenshotBuf []byte // Buffer to hold the screenshot

	err := chromedp.Run(ctx,
		chromedp.Navigate(loginPageURL),
		chromedp.WaitVisible(`#email-address`, chromedp.ByQuery),
		chromedp.SendKeys(`#email-address`, username, chromedp.ByQuery),
		chromedp.SendKeys(`#password`, password, chromedp.ByQuery),
		// this work: verify by wrong password
		chromedp.WaitEnabled("sign-in-btn", chromedp.ByID),
		chromedp.Click("sign-in-btn", chromedp.ByID),

		// Wait for potential redirects and JS challenges
		chromedp.Sleep(10*time.Second),
		// chromedp.WaitVisible(`your-cases`, chromedp.ByID),

		// --- ALWAYS TAKE A SCREENSHOT AFTER THE WAIT ---
		chromedp.FullScreenshot(&screenshotBuf, 90),

		// 4. Get all cookies after the page has loaded successfully
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)

	// --- ALWAYS SAVE THE SCREENSHOT FOR REVIEW ---
	if err := os.WriteFile(
		fmt.Sprintf("final_page_view_%s.png", time.Now().Format("2006-01-02T15-04-05")),
		screenshotBuf, 0644); err != nil {
		fmt.Printf("Failed to save screenshot: %v", err)
	}

	if err != nil {
		return "", fmt.Errorf("login automation failed: %w", err)
	}

	// 5. Process the cookies after the browser tasks are complete
	fmt.Printf("check cookies\n")
	for _, cookie := range cookies {
		fmt.Printf("cookie: %s=%s\n", cookie.Name, cookie.Value)
	}
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
