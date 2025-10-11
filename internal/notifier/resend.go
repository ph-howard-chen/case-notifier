package notifier

// ResendClient handles email notifications via Resend API
type ResendClient struct {
	apiKey string
}

// NewResendClient creates a new Resend client
func NewResendClient(apiKey string) *ResendClient {
	return &ResendClient{
		apiKey: apiKey,
	}
}

// SendEmail sends an email notification
func (r *ResendClient) SendEmail(to, subject, body string) error {
	// TODO: implement email sending
	return nil
}
