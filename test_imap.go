package main

import (
	"log"
	"os"
	"time"

	"github.com/phhowardchen/case-tracker/internal/email"
)

// source .env
// export EMAIL_IMAP_SERVER EMAIL_USERNAME EMAIL_PASSWORD EMAIL_2FA_SENDER EMAIL_2FA_TIMEOUT
// go build -o test_imap test_imap.go
// ./test_imap

func main() {

	emailIMAPServer := os.Getenv("EMAIL_IMAP_SERVER")
	emailUsername := os.Getenv("EMAIL_USERNAME")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	email2FASender := os.Getenv("EMAIL_2FA_SENDER")
	email2FATimeout := os.Getenv("EMAIL_2FA_TIMEOUT")

	if emailIMAPServer != "" && emailUsername != "" && emailPassword != "" && email2FASender != "" && email2FATimeout != "" {
		log.Printf("2FA: Automated email fetch enabled")
		log.Printf("  Email Server: %s", emailIMAPServer)
		log.Printf("  Email Account: %s", emailUsername)
	}
	// Create IMAP client for automated 2FA
	imapClient := email.NewIMAPClient(emailIMAPServer, emailUsername, emailPassword)

	log.Printf("Fetching 2FA code from email (sender: %s)...", email2FASender)
	timeout, err := time.ParseDuration(email2FATimeout)
	if err != nil {
		log.Fatalf("invalid EMAIL_2FA_TIMEOUT: %v", err)
	}
	code, err := imapClient.FetchLatest2FACode(email2FASender, timeout)
	if err != nil {
		log.Fatalf("Failed to fetch 2FA code from email: %v\n", err)
	}
	log.Printf("Fetched 2FA code: %s\n", code)
}
