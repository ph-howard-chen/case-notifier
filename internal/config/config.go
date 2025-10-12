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
}

// Load loads configuration from environment variables (multi-case aware)
func Load() (*Config, error) {
   cfg := &Config{
	   USCISCookie:    os.Getenv("USCIS_COOKIE"),
	   ResendAPIKey:   os.Getenv("RESEND_API_KEY"),
	   RecipientEmail: os.Getenv("RECIPIENT_EMAIL"),
   }

   // Parse CASE_IDS as comma-separated list
   caseIDsStr := os.Getenv("CASE_IDS")
   if caseIDsStr != "" {
	   ids := strings.Split(caseIDsStr, ",")
	   for i, id := range ids {
		   ids[i] = strings.TrimSpace(id)
	   }
	   cfg.CaseIDs = ids
   }

   // Validate required fields
   if cfg.USCISCookie == "" {
	   return nil, fmt.Errorf("USCIS_COOKIE environment variable is required")
   }
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

   return cfg, nil
}
