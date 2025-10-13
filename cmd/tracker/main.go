package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/phhowardchen/case-tracker/internal/config"
	"github.com/phhowardchen/case-tracker/internal/notifier"
	"github.com/phhowardchen/case-tracker/internal/storage"
	"github.com/phhowardchen/case-tracker/internal/uscis"
)

// CaseStatusFetcher is an interface for fetching case status
// Implemented by both Client (HTTP) and BrowserClient (chromedp)
type CaseStatusFetcher interface {
	FetchCaseStatus(caseID string) (map[string]interface{}, error)
}

func main() {
	log.Println("USCIS Case Tracker starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded successfully")
	log.Printf("  Case IDs: %v", cfg.CaseIDs)
	log.Printf("  Recipient: %s", cfg.RecipientEmail)
	log.Printf("  Poll Interval: %v", cfg.PollInterval)
	log.Printf("  State Directory: %s", cfg.StateFileDir)

	// Initialize USCIS client based on authentication mode
	var fetcher CaseStatusFetcher

	if cfg.AutoLogin {
		log.Println("  Authentication: Auto-login mode (chromedp browser)")
		log.Printf("  Username: %s", cfg.USCISUsername)
		browserClient, err := uscis.NewBrowserClient(cfg.USCISUsername, cfg.USCISPassword)
		if err != nil {
			log.Fatalf("Failed to create browser client: %v", err)
		}
		defer browserClient.Close()
		log.Println("  Successfully logged in with browser")
		fetcher = browserClient
	} else {
		log.Println("  Authentication: Manual cookie mode (HTTP client)")
		fetcher = uscis.NewClient(cfg.USCISCookie)
	}

	// Initialize email client
	emailClient := notifier.NewResendClient(cfg.ResendAPIKey)

	// Create ticker for polling
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run initial check immediately for all cases
	log.Printf("Running initial check for %d case(s)...", len(cfg.CaseIDs))
	for _, caseID := range cfg.CaseIDs {
		if err := checkAndNotifyCase(fetcher, emailClient, cfg, caseID); err != nil {
			log.Printf("Error in initial check for case %s: %v", caseID, err)
			// Check for auth failure - if so, stop everything
			if _, ok := err.(*uscis.ErrAuthenticationFailed); ok {
				log.Fatalf("Authentication failed, cannot continue")
			}
		}
	}

	// Main loop
	for {
		select {
		case <-ticker.C:
			log.Printf("Polling %d case(s)...", len(cfg.CaseIDs))
			for _, caseID := range cfg.CaseIDs {
				if err := checkAndNotifyCase(fetcher, emailClient, cfg, caseID); err != nil {
					log.Printf("Error checking case %s: %v", caseID, err)
					// Check for auth failure - if so, stop everything
					if _, ok := err.(*uscis.ErrAuthenticationFailed); ok {
						log.Fatalf("Authentication failed, cannot continue")
					}
				}
			}
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			return
		}
	}
}

func checkAndNotifyCase(fetcher CaseStatusFetcher, emailClient *notifier.ResendClient, cfg *config.Config, caseID string) error {
	log.Printf("Fetching case status for %s...", caseID)

	// Create storage for this specific case
	stateStorage := storage.NewFileStorage(cfg.StateFileDir, caseID)

	// Load previous state for this case
	previousState, err := stateStorage.Load()
	if err != nil {
		log.Printf("Warning: Failed to load previous state for %s: %v", caseID, err)
	}

	// Fetch case status
	status, err := fetcher.FetchCaseStatus(caseID)
	if err != nil {
		// Check if it's an authentication error
		if _, ok := err.(*uscis.ErrAuthenticationFailed); ok {
			log.Printf("Authentication failed! Cookie may have expired.")
			// Send alert email
			subject := "USCIS Case Tracker - Cookie Expired"
			body := fmt.Sprintf(`
				<h2>Authentication Failed</h2>
				<p>The USCIS cookie has expired. Please update the USCIS_COOKIE environment variable with a fresh cookie.</p>
				<p>Error: %v</p>
			`, err)
			if sendErr := emailClient.SendEmail(cfg.RecipientEmail, subject, body); sendErr != nil {
				log.Printf("Failed to send alert email: %v", sendErr)
			}
			return fmt.Errorf("authentication failed, exiting: %w", err)
		}
		return fmt.Errorf("failed to fetch case status: %w", err)
	}

	log.Printf("Case status fetched successfully")

	// Save current state to storage
	if err := stateStorage.Save(status); err != nil {
		log.Printf("Warning: Failed to save state: %v", err)
	}

	// Detect changes
	changes := uscis.DetectChanges(previousState, status)

	// Determine if we should send email
	isFirstRun := previousState == nil
	hasChanges := len(changes) > 0

	if isFirstRun {
		log.Printf("[%s] First run - sending initial status email", caseID)
		subject := fmt.Sprintf("USCIS Case Tracker - Initial Status for %s", caseID)
		body := formatInitialStatusEmail(status, caseID)
		if err := emailClient.SendEmail(cfg.RecipientEmail, subject, body); err != nil {
			return fmt.Errorf("failed to send initial email: %w", err)
		}
		log.Printf("[%s] Initial status email sent successfully", caseID)
	} else if hasChanges {
		log.Printf("[%s] Changes detected: %d fields changed", caseID, len(changes))
		subject := fmt.Sprintf("USCIS Case Status Update - %s", caseID)
		body := formatChangeNotificationEmail(changes, status, caseID)
		if err := emailClient.SendEmail(cfg.RecipientEmail, subject, body); err != nil {
			return fmt.Errorf("failed to send change notification: %w", err)
		}
		log.Printf("[%s] Change notification email sent successfully", caseID)
	} else {
		log.Printf("[%s] No changes detected - skipping email notification", caseID)
	}

	return nil
}

func formatInitialStatusEmail(status map[string]interface{}, caseID string) string {
	jsonBytes, _ := json.MarshalIndent(status, "", "  ")

	html := fmt.Sprintf(`
		<h2>USCIS Case Tracker - Initial Status</h2>
		<p><strong>Case ID:</strong> %s</p>
		<p>This is the first status check for your case. Future emails will only be sent when changes are detected.</p>
		<h3>Current Status:</h3>
		<pre style="background-color: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-family: monospace;">%s</pre>
		<p><small>This email was sent by USCIS Case Tracker</small></p>
	`, caseID, string(jsonBytes))

	return html
}

func formatChangeNotificationEmail(changes []uscis.Change, status map[string]interface{}, caseID string) string {
	jsonBytes, _ := json.MarshalIndent(status, "", "  ")

	// Build changes list
	changesHTML := "<ul>"
	for _, change := range changes {
		if change.OldValue == nil {
			changesHTML += fmt.Sprintf("<li><strong>%s</strong>: <span style='color: green;'>%v</span> (new field)</li>", change.Field, change.NewValue)
		} else if change.NewValue == nil {
			changesHTML += fmt.Sprintf("<li><strong>%s</strong>: <span style='color: red;'>%v</span> (removed)</li>", change.Field, change.OldValue)
		} else {
			changesHTML += fmt.Sprintf("<li><strong>%s</strong>: <span style='color: red;'>%v</span> → <span style='color: green;'>%v</span></li>", change.Field, change.OldValue, change.NewValue)
		}
	}
	changesHTML += "</ul>"

	html := fmt.Sprintf(`
		<h2>USCIS Case Status Update Detected!</h2>
		<p><strong>Case ID:</strong> %s</p>
		<p>The following changes were detected in your case status:</p>
		%s
		<h3>Full Current Status:</h3>
		<pre style="background-color: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-family: monospace;">%s</pre>
		<p><small>This email was sent by USCIS Case Tracker</small></p>
	`, caseID, changesHTML, string(jsonBytes))

	return html
}
