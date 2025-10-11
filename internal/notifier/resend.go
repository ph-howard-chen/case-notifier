package notifier

import (
	"fmt"

	"github.com/resend/resend-go/v2"
)

// ResendClient handles email notifications via Resend API
type ResendClient struct {
	client *resend.Client
	from   string
}

// NewResendClient creates a new Resend client
func NewResendClient(apiKey string) *ResendClient {
	return &ResendClient{
		client: resend.NewClient(apiKey),
		from:   "Case Tracker Test <onboarding@resend.dev>",
	}
}

// SendEmail sends an email notification
func (r *ResendClient) SendEmail(to, subject, body string) error {
	params := &resend.SendEmailRequest{
		From:    r.from,
		To:      []string{to},
		Subject: subject,
		Html:    body,
	}

	sent, err := r.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	if sent == nil {
		return fmt.Errorf("email send returned nil response")
	}

	return nil
}
