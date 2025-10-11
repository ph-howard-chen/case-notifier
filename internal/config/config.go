package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds the application configuration
type Config struct {
	USCISCookie    string
	CaseID         string
	ResendAPIKey   string
	RecipientEmail string
	PollInterval   time.Duration
	StateFilePath  string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		USCISCookie:    os.Getenv("USCIS_COOKIE"),
		CaseID:         os.Getenv("CASE_ID"),
		ResendAPIKey:   os.Getenv("RESEND_API_KEY"),
		RecipientEmail: os.Getenv("RECIPIENT_EMAIL"),
		StateFilePath:  os.Getenv("STATE_FILE_PATH"),
	}

	// Validate required fields
	if cfg.USCISCookie == "" {
		return nil, fmt.Errorf("USCIS_COOKIE environment variable is required")
	}
	if cfg.CaseID == "" {
		return nil, fmt.Errorf("CASE_ID environment variable is required")
	}
	if cfg.ResendAPIKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY environment variable is required")
	}
	if cfg.RecipientEmail == "" {
		return nil, fmt.Errorf("RECIPIENT_EMAIL environment variable is required")
	}

	// Set defaults for optional fields
	if cfg.StateFilePath == "" {
		cfg.StateFilePath = "/tmp/case-tracker-state.json"
	}

	// Parse poll interval with default
	pollIntervalStr := os.Getenv("POLL_INTERVAL")
	if pollIntervalStr == "" {
		cfg.PollInterval = 5 * time.Minute
	} else {
		interval, err := time.ParseDuration(pollIntervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = interval
	}

	return cfg, nil
}
