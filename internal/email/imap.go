package email

import (
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// IMAPClient handles fetching 2FA codes from email
type IMAPClient struct {
	server   string
	username string
	password string
}

// NewIMAPClient creates a new IMAP client
func NewIMAPClient(server, username, password string) *IMAPClient {
	return &IMAPClient{
		server:   server,
		username: username,
		password: password,
	}
}

// FetchLatest2FACode fetches the latest 2FA verification code from email
// Polls the inbox until a code is found or timeout is reached
func (c *IMAPClient) FetchLatest2FACode(senderEmail string, maxWaitTime time.Duration) (string, error) {
	deadline := time.Now().Add(maxWaitTime)
	pollInterval := 5 * time.Second

	log.Printf("Waiting for 2FA email from %s (timeout: %v)...", senderEmail, maxWaitTime)

	for time.Now().Before(deadline) {
		code, err := c.tryFetchCode(senderEmail)
		if err == nil && code != "" {
			log.Printf("Successfully retrieved 2FA code: %s", code)
			return code, nil
		}

		// If error is not "not found", return it
		if err != nil && !strings.Contains(err.Error(), "no 2FA email found") {
			log.Printf("Error fetching 2FA code, retry...: %v", err)
			continue
		}

		// Wait before retrying
		remaining := time.Until(deadline)
		if remaining < pollInterval {
			break
		}
		log.Printf("No 2FA email yet, waiting %v before retry...", pollInterval)
		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout: no 2FA email received within %v", maxWaitTime)
}

// tryFetchCode attempts to fetch a 2FA code from recent emails
func (c *IMAPClient) tryFetchCode(senderEmail string) (string, error) {
	// Connect to IMAP server
	imapClient, err := client.DialTLS(c.server, nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer imapClient.Logout()

	// Login
	if err := imapClient.Login(c.username, c.password); err != nil {
		return "", fmt.Errorf("failed to login to IMAP: %w", err)
	}

	// Select INBOX
	_, err = imapClient.Select("INBOX", false)
	if err != nil {
		return "", fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Search for recent emails from sender (last 5 minutes)
	fiveMinutesAgo := time.Now().Add(-5 * time.Minute)
	criteria := imap.NewSearchCriteria()
	criteria.SentSince = fiveMinutesAgo
	criteria.Header.Add("FROM", senderEmail)

	uids, err := imapClient.Search(criteria)
	if err != nil {
		return "", fmt.Errorf("failed to search emails: %w", err)
	}

	if len(uids) == 0 {
		return "", fmt.Errorf("no 2FA email found from %s in last 5 minutes", senderEmail)
	}

	// Get the most recent email (last UID)
	latestUID := uids[len(uids)-1]

	// Fetch the email body
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(latestUID)

	messages := make(chan *imap.Message, 1)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem()}

	done := make(chan error, 1)
	go func() {
		done <- imapClient.Fetch(seqSet, items, messages)
	}()

	// Wait for message
	msg := <-messages
	if msg == nil {
		return "", fmt.Errorf("failed to fetch email message")
	}

	// Wait for fetch to complete
	if err := <-done; err != nil {
		return "", fmt.Errorf("fetch error: %w", err)
	}

	// Read the message body
	literal := msg.GetBody(section)
	if literal == nil {
		return "", fmt.Errorf("email body is empty")
	}

	bodyBytes, err := io.ReadAll(literal)
	if err != nil {
		return "", fmt.Errorf("failed to read email body: %w", err)
	}
	bodyText := string(bodyBytes)

	// Extract 6-digit code from email body
	code, err := extract2FACode(bodyText)
	if err != nil {
		return "", fmt.Errorf("failed to extract 2FA code from email: %w", err)
	}

	return code, nil
}

// extract2FACode extracts a 6-digit verification code from email text
func extract2FACode(text string) (string, error) {
	// Look for 6-digit number patterns
	re := regexp.MustCompile(`\bPlease enter this secure verification code:\s*(\d{6})\b`)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return "", fmt.Errorf("no 6-digit code found in email body")
	}

	// Return the first match (usually the verification code)
	code := matches[0][1]
	return code, nil
}
