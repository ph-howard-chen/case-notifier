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

func main() {
	log.Println("USCIS Case Tracker starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded successfully")
	log.Printf("  Case ID: %s", cfg.CaseID)
	log.Printf("  Recipient: %s", cfg.RecipientEmail)
	log.Printf("  Poll Interval: %v", cfg.PollInterval)

	// Initialize clients
	uscisClient := uscis.NewClient(cfg.USCISCookie)
	emailClient := notifier.NewResendClient(cfg.ResendAPIKey)
	stateStorage := storage.NewFileStorage(cfg.StateFilePath)

	log.Printf("  State File: %s", cfg.StateFilePath)

	// Load previous state
	previousState, err := stateStorage.Load()
	if err != nil {
		log.Printf("Warning: Failed to load previous state: %v", err)
	} else if previousState == nil {
		log.Printf("No previous state found - this is the first run")
	} else {
		log.Printf("Loaded previous state successfully")
	}

	// Create ticker for polling
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run initial check immediately
	if err := checkAndNotify(uscisClient, emailClient, stateStorage, cfg, previousState); err != nil {
		log.Printf("Error in initial check: %v", err)
		// Update previous state even if notification failed
		if err.Error() != "authentication failed, exiting: authentication failed: received status code 401 (cookie may have expired)" {
			previousState, _ = stateStorage.Load()
		}
	} else {
		// Update previous state after successful check
		previousState, _ = stateStorage.Load()
	}

	// Main loop
	for {
		select {
		case <-ticker.C:
			if err := checkAndNotify(uscisClient, emailClient, stateStorage, cfg, previousState); err != nil {
				log.Printf("Error checking case status: %v", err)
				// Update previous state even if notification failed
				if err.Error() != "authentication failed, exiting: authentication failed: received status code 401 (cookie may have expired)" {
					previousState, _ = stateStorage.Load()
				}
			} else {
				// Update previous state after successful check
				previousState, _ = stateStorage.Load()
			}
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			return
		}
	}
}

func checkAndNotify(uscisClient *uscis.Client, emailClient *notifier.ResendClient, stateStorage storage.Storage, cfg *config.Config, previousState map[string]interface{}) error {
	log.Printf("Fetching case status for %s...", cfg.CaseID)

	// Fetch case status
	status, err := uscisClient.FetchCaseStatus(cfg.CaseID)
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
		log.Printf("First run - sending initial status email")
		subject := fmt.Sprintf("USCIS Case Tracker - Initial Status for %s", cfg.CaseID)
		body := formatInitialStatusEmail(status)
		if err := emailClient.SendEmail(cfg.RecipientEmail, subject, body); err != nil {
			return fmt.Errorf("failed to send initial email: %w", err)
		}
		log.Printf("Initial status email sent successfully")
	} else if hasChanges {
		log.Printf("Changes detected: %d fields changed", len(changes))
		subject := fmt.Sprintf("USCIS Case Status Update - %s", cfg.CaseID)
		body := formatChangeNotificationEmail(changes, status)
		if err := emailClient.SendEmail(cfg.RecipientEmail, subject, body); err != nil {
			return fmt.Errorf("failed to send change notification: %w", err)
		}
		log.Printf("Change notification email sent successfully")
	} else {
		log.Printf("No changes detected - skipping email notification")
	}

	return nil
}

func formatInitialStatusEmail(status map[string]interface{}) string {
	jsonBytes, _ := json.MarshalIndent(status, "", "  ")

	html := fmt.Sprintf(`
		<h2>USCIS Case Tracker - Initial Status</h2>
		<p>This is the first status check for your case. Future emails will only be sent when changes are detected.</p>
		<h3>Current Status:</h3>
		<pre style="background-color: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-family: monospace;">%s</pre>
		<p><small>This email was sent by USCIS Case Tracker</small></p>
	`, string(jsonBytes))

	return html
}

func formatChangeNotificationEmail(changes []uscis.Change, status map[string]interface{}) string {
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
		<p>The following changes were detected in your case status:</p>
		%s
		<h3>Full Current Status:</h3>
		<pre style="background-color: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-family: monospace;">%s</pre>
		<p><small>This email was sent by USCIS Case Tracker</small></p>
	`, changesHTML, string(jsonBytes))

	return html
}
