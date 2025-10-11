package config

// Config holds the application configuration
type Config struct {
	USCISCookie    string
	CaseID         string
	ResendAPIKey   string
	RecipientEmail string
	PollInterval   string
	StateFilePath  string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// TODO: implement configuration loading
	return &Config{}, nil
}
