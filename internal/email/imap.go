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
// The senderEmail parameter is kept for interface compatibility but not used -
// we search for USCIS emails by checking sender/subject keywords instead
func (c *IMAPClient) FetchLatest2FACode(senderEmail string, maxWaitTime time.Duration) (string, error) {
	deadline := time.Now().Add(maxWaitTime)
	pollInterval := 5 * time.Second

	log.Printf("Waiting for 2FA email (timeout: %v)...", maxWaitTime)

	for time.Now().Before(deadline) {
		code, err := c.tryFetchCode()
		if err == nil && code != "" {
			log.Printf("Successfully retrieved 2FA code: %s", code)
			return code, nil
		}

		// If error is not "not found", log and retry
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
func (c *IMAPClient) tryFetchCode() (string, error) {
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
	mbox, err := imapClient.Select("INBOX", false)
	if err != nil {
		return "", fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Get the last 50 messages (more reliable than time-based search)
	maxToCheck := uint32(50)
	if mbox.Messages < maxToCheck {
		maxToCheck = mbox.Messages
	}

	if maxToCheck == 0 {
		return "", fmt.Errorf("no emails in INBOX")
	}

	firstUID := mbox.Messages - maxToCheck + 1
	lastUID := mbox.Messages

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(firstUID, lastUID)

	// Fetch email headers and body for these messages
	messages := make(chan *imap.Message, maxToCheck)
	done := make(chan error, 1)

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		(&imap.BodySectionName{}).FetchItem(),
	}

	go func() {
		done <- imapClient.Fetch(seqSet, items, messages)
	}()

	// Collect all messages
	var allMessages []*imap.Message
	for msg := range messages {
		if msg != nil {
			allMessages = append(allMessages, msg)
		}
	}

	if err := <-done; err != nil {
		return "", fmt.Errorf("fetch error: %w", err)
	}

	// Check messages from most recent to oldest
	for i := len(allMessages) - 1; i >= 0; i-- {
		msg := allMessages[i]
		if msg == nil {
			continue
		}

		// Check if this is a USCIS email by sender/subject
		if msg.Envelope != nil {
			var fromAddr string
			if len(msg.Envelope.From) > 0 {
				fromAddr = msg.Envelope.From[0].Address()
			}
			subject := msg.Envelope.Subject

			// Check if this is from USCIS (flexible matching)
			isUSCIS := strings.Contains(strings.ToLower(fromAddr), "uscis") ||
				strings.Contains(strings.ToLower(subject), "verification") ||
				strings.Contains(strings.ToLower(subject), "myaccount") ||
				strings.Contains(strings.ToLower(subject), "secure")

			if !isUSCIS {
				continue
			}

			// Found a USCIS email, try to extract code
			section := &imap.BodySectionName{}
			literal := msg.GetBody(section)
			if literal == nil {
				continue
			}

			bodyBytes, err := io.ReadAll(literal)
			if err != nil {
				continue
			}

			code, err := extract2FACode(string(bodyBytes))
			if err == nil {
				log.Printf("Found 2FA code from: %s", fromAddr)
				return code, nil
			}
		}
	}

	return "", fmt.Errorf("no 2FA email found from USCIS in last %d emails", maxToCheck)
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
