package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	USCISCookie    string
	CaseIDs        []string
	ResendAPIKey   string
	RecipientEmail string
	PollInterval   time.Duration
	StateFileDir   string

	// Auto-login configuration
	AutoLogin     bool
	USCISUsername string
	USCISPassword string

	// Email 2FA configuration (optional - for automated 2FA)
	EmailIMAPServer  string
	EmailUsername    string
	EmailPassword    string
	Email2FASender   string
	Email2FATimeout  time.Duration
}

// Load loads configuration from environment variables (multi-case aware)
func Load() (*Config, error) {
	cfg := &Config{
		USCISCookie:     os.Getenv("USCIS_COOKIE"),
		ResendAPIKey:    os.Getenv("RESEND_API_KEY"),
		RecipientEmail:  os.Getenv("RECIPIENT_EMAIL"),
		USCISUsername:   os.Getenv("USCIS_USERNAME"),
		USCISPassword:   os.Getenv("USCIS_PASSWORD"),
		EmailIMAPServer: os.Getenv("EMAIL_IMAP_SERVER"),
		EmailUsername:   os.Getenv("EMAIL_USERNAME"),
		EmailPassword:   os.Getenv("EMAIL_PASSWORD"),
		Email2FASender:  os.Getenv("EMAIL_2FA_SENDER"),
	}

	// Parse AUTO_LOGIN flag
	autoLoginStr := strings.ToLower(os.Getenv("AUTO_LOGIN"))
	cfg.AutoLogin = autoLoginStr == "true" || autoLoginStr == "1" || autoLoginStr == "yes"

	// Parse CASE_IDS as comma-separated list
	caseIDsStr := os.Getenv("CASE_IDS")
	if caseIDsStr != "" {
		ids := strings.Split(caseIDsStr, ",")
		for i, id := range ids {
			ids[i] = strings.TrimSpace(id)
		}
		cfg.CaseIDs = ids
	}

	// Validate authentication method (either manual cookie or auto-login)
	if cfg.AutoLogin {
		// Auto-login mode requires username and password
		if cfg.USCISUsername == "" {
			return nil, fmt.Errorf("USCIS_USERNAME environment variable is required when AUTO_LOGIN=true")
		}
		if cfg.USCISPassword == "" {
			return nil, fmt.Errorf("USCIS_PASSWORD environment variable is required when AUTO_LOGIN=true")
		}
	} else {
		// Manual cookie mode requires USCIS_COOKIE
		if cfg.USCISCookie == "" {
			return nil, fmt.Errorf("USCIS_COOKIE environment variable is required when AUTO_LOGIN is not enabled")
		}
	}

	// Validate other required fields
	if len(cfg.CaseIDs) == 0 || (len(cfg.CaseIDs) == 1 && cfg.CaseIDs[0] == "") {
		return nil, fmt.Errorf("CASE_IDS environment variable is required (comma-separated list)")
	}
	if cfg.ResendAPIKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY environment variable is required")
	}
	if cfg.RecipientEmail == "" {
		return nil, fmt.Errorf("RECIPIENT_EMAIL environment variable is required")
	}

	// Set default for state file directory
	stateFileDir := os.Getenv("STATE_FILE_DIR")
	if stateFileDir == "" {
		stateFileDir = "/tmp/case-tracker-states/"
	}
	cfg.StateFileDir = stateFileDir

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

	// Parse email 2FA timeout with default
	email2FATimeoutStr := os.Getenv("EMAIL_2FA_TIMEOUT")
	if email2FATimeoutStr == "" {
		cfg.Email2FATimeout = 5 * time.Minute
	} else {
		timeout, err := time.ParseDuration(email2FATimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid EMAIL_2FA_TIMEOUT: %w", err)
		}
		cfg.Email2FATimeout = timeout
	}

	// Validate email settings if any are provided
	emailFieldsSet := []bool{
		cfg.EmailIMAPServer != "",
		cfg.EmailUsername != "",
		cfg.EmailPassword != "",
		cfg.Email2FASender != "",
	}
	someEmailFieldsSet := false
	allEmailFieldsSet := true
	for _, set := range emailFieldsSet {
		if set {
			someEmailFieldsSet = true
		} else {
			allEmailFieldsSet = false
		}
	}

	// If any email field is set, all must be set
	if someEmailFieldsSet && !allEmailFieldsSet {
		return nil, fmt.Errorf("if any email settings are provided, all of EMAIL_IMAP_SERVER, EMAIL_USERNAME, EMAIL_PASSWORD, and EMAIL_2FA_SENDER must be set")
	}

	return cfg, nil
}
