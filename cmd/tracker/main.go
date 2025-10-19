package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/phhowardchen/case-tracker/internal/config"
	"github.com/phhowardchen/case-tracker/internal/email"
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
	log.Printf("USCIS Case Tracker starting...")

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

	// Start HTTP health check server for Cloud Run
	// Cloud Run requires services to listen on PORT (default 8080)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "USCIS Case Tracker is running")
		})

		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "OK")
		})

		log.Printf("Starting HTTP health check server on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Initialize email client early so we can send notifications
	emailClient := notifier.NewResendClient(cfg.ResendAPIKey)

	// Initialize USCIS client based on authentication mode
	var fetcher CaseStatusFetcher

	if cfg.AutoLogin {
		log.Printf("Authentication: Auto-login mode (chromedp browser)")

		// Check if email 2FA settings are configured
		var browserClient *uscis.BrowserClient
		if cfg.EmailIMAPServer != "" && cfg.EmailUsername != "" && cfg.EmailPassword != "" {
			log.Printf("2FA: Automated email fetch enabled")
			log.Printf("  Email Server: %s", cfg.EmailIMAPServer)
			log.Printf("  Email Account: %s", cfg.EmailUsername)
			log.Printf("  2FA Sender: MyAccount@uscis.dhs.gov (hardcoded)")
			log.Printf("  2FA Timeout: 10m (hardcoded)")

			// Create IMAP client for automated 2FA
			imapClient := email.NewIMAPClient(cfg.EmailIMAPServer, cfg.EmailUsername, cfg.EmailPassword)

			// Create browser client with email support (hardcoded 2FA settings)
			browserClient, err = uscis.NewBrowserClientWithEmail(
				cfg.USCISUsername,
				cfg.USCISPassword,
				imapClient,
				"MyAccount@uscis.dhs.gov", // Hardcoded 2FA sender
				10*time.Minute,            // Hardcoded 2FA timeout
			)
			if err != nil {
				log.Printf("CRITICAL: Failed to create browser client: %v", err)
				log.Printf("This could indicate:")
				log.Printf("  - Incorrect USCIS username or password")
				log.Printf("  - Account locked due to too many failed attempts")
				log.Printf("  - USCIS website issues")
				log.Printf("")
				log.Printf("Sending email notification and exiting to prevent account lockout.")

				// Send email notification about authentication failure
				sendAuthFailureEmail(emailClient, cfg.RecipientEmail, err, "browser initialization")

				log.Printf("Fix credentials and redeploy to retry.")
				os.Exit(1)
			}
		} else {
			log.Printf("2FA: Manual stdin input (email settings not configured)")
			// Create browser client without email support (falls back to stdin for 2FA)
			browserClient, err = uscis.NewBrowserClient(cfg.USCISUsername, cfg.USCISPassword)
			if err != nil {
				log.Printf("CRITICAL: Failed to create browser client: %v", err)
				log.Printf("This could indicate:")
				log.Printf("  - Incorrect USCIS username or password")
				log.Printf("  - Account locked due to too many failed attempts")
				log.Printf("  - USCIS website issues")
				log.Printf("")
				log.Printf("Sending email notification and exiting to prevent account lockout.")

				// Send email notification about authentication failure
				sendAuthFailureEmail(emailClient, cfg.RecipientEmail, err, "browser initialization")

				log.Printf("Fix credentials and redeploy to retry.")
				os.Exit(1)
			}
		}

		defer browserClient.Close()
		log.Printf("Successfully logged in with browser")
		fetcher = browserClient
	} else {
		log.Printf("Authentication: Manual cookie mode (HTTP client)")
		fetcher = uscis.NewClient(cfg.USCISCookie)
	}

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
			log.Printf("[%s] Error during initial check: %v", caseID, err)
			// Don't exit - continue running and retry on next poll
		}
	}

	// Main loop
	for {
		select {
		case <-ticker.C:
			log.Printf("Polling %d case(s)...", len(cfg.CaseIDs))
			for _, caseID := range cfg.CaseIDs {
				if err := checkAndNotifyCase(fetcher, emailClient, cfg, caseID); err != nil {
					log.Printf("[%s] Error during poll: %v", caseID, err)
					// Continue checking other cases even if one fails
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
		// Check if it's an authentication error (both manual cookie and browser auto-login modes)
		if _, ok := err.(*uscis.ErrAuthenticationFailed); ok {
			log.Printf("Authentication failed! Sending email notification...")
			// Send alert email (works for both modes)
			sendAuthFailureEmail(emailClient, cfg.RecipientEmail, err, "polling")
			return fmt.Errorf("authentication failed: %w", err)
		}

		return fmt.Errorf("failed to fetch case status: %w", err)
	}

	log.Printf("Case status fetched successfully")

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

	// Save current state to storage if has first run or has changes
	if isFirstRun || hasChanges {
		if err := stateStorage.Save(status); err != nil {
			log.Printf("Warning: Failed to save state: %v", err)
		}
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

// sendAuthFailureEmail sends an email notification when authentication fails
func sendAuthFailureEmail(emailClient *notifier.ResendClient, recipientEmail string, err error, context string) {
	subject := "USCIS Case Tracker - Authentication Failed"
	body := fmt.Sprintf(`
		<h2>⚠️ Authentication Failed</h2>
		<p><strong>Context:</strong> %s</p>
		<p><strong>Error:</strong> %v</p>

		<h3>What this means:</h3>
		<ul>
			<li><strong>Browser auto-login mode:</strong> USCIS username/password may be incorrect, or your account may be locked</li>
			<li><strong>Manual cookie mode:</strong> Your USCIS session cookie has expired</li>
			<li><strong>Session refresh:</strong> The service attempted to re-authenticate but failed</li>
		</ul>

		<h3>What to do:</h3>
		<ol>
			<li><strong>Check your credentials:</strong> Verify USCIS username and password are correct</li>
			<li><strong>Check account status:</strong> Login to https://my.uscis.gov to verify your account is not locked</li>
			<li><strong>Update secrets:</strong> If using GCP Secret Manager, update the secrets:
				<pre style="background-color: #f5f5f5; padding: 10px; border-radius: 5px;">
gcloud secrets versions add uscis-username --data-file=- --project=your-project-id
gcloud secrets versions add uscis-password --data-file=- --project=your-project-id</pre>
			</li>
			<li><strong>Redeploy:</strong> Redeploy the service to pick up new credentials</li>
		</ol>

		<p><strong>Note:</strong> The service will automatically exit to prevent account lockout from repeated failed login attempts.</p>

		<p><small>This alert was sent by USCIS Case Tracker</small></p>
	`, context, err)

	if sendErr := emailClient.SendEmail(recipientEmail, subject, body); sendErr != nil {
		log.Printf("Failed to send authentication failure alert email: %v", sendErr)
	} else {
		log.Printf("Authentication failure alert email sent successfully to %s", recipientEmail)
	}
}
