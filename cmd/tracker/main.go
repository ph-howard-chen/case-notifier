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

	// Create ticker for polling
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run initial check immediately
	if err := checkAndNotify(uscisClient, emailClient, cfg); err != nil {
		log.Printf("Error in initial check: %v", err)
	}

	// Main loop
	for {
		select {
		case <-ticker.C:
			if err := checkAndNotify(uscisClient, emailClient, cfg); err != nil {
				log.Printf("Error checking case status: %v", err)
			}
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			return
		}
	}
}

func checkAndNotify(uscisClient *uscis.Client, emailClient *notifier.ResendClient, cfg *config.Config) error {
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

	// Format status as email
	subject := fmt.Sprintf("USCIS Case Status - %s", cfg.CaseID)
	body, err := formatStatusEmail(status)
	if err != nil {
		return fmt.Errorf("failed to format email: %w", err)
	}

	// Send email
	log.Printf("Sending email to %s...", cfg.RecipientEmail)
	if err := emailClient.SendEmail(cfg.RecipientEmail, subject, body); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email sent successfully")
	return nil
}

func formatStatusEmail(status map[string]interface{}) (string, error) {
	// Pretty print the JSON
	jsonBytes, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	html := fmt.Sprintf(`
		<h2>USCIS Case Status Update</h2>
		<p>Current case status:</p>
		<pre style="background-color: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto;">%s</pre>
		<p><small>This email was sent by USCIS Case Tracker</small></p>
	`, string(jsonBytes))

	return html, nil
}
