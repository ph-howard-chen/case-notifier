package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Storage interface for persisting case status
type Storage interface {
	Load() (map[string]interface{}, error)
	Save(data map[string]interface{}) error
}

// FileStorage implements Storage using a JSON file with timestamps
type FileStorage struct {
	stateDir string
	caseID   string
}

// NewFileStorage creates a new file-based storage for a specific case
func NewFileStorage(stateDir, caseID string) *FileStorage {
	return &FileStorage{
		stateDir: stateDir,
		caseID:   caseID,
	}
}

// Load loads the most recent state file for this case
func (f *FileStorage) Load() (map[string]interface{}, error) {
	// Check if directory exists
	if _, err := os.Stat(f.stateDir); os.IsNotExist(err) {
		// Directory doesn't exist - first run
		return nil, nil
	}

	// Find all state files for this case
	pattern := filepath.Join(f.stateDir, f.caseID+"_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search for state files: %w", err)
	}

	if len(matches) == 0 {
		// No previous state files - first run for this case
		return nil, nil
	}

	// Sort by filename (timestamp is in filename) - most recent first
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})

	// Load the most recent file
	mostRecentFile := matches[0]
	data, err := os.ReadFile(mostRecentFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file %s: %w", mostRecentFile, err)
	}

	// Parse JSON
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", mostRecentFile, err)
	}

	return state, nil
}

// Save saves the current state to a new timestamped file
func (f *FileStorage) Save(data map[string]interface{}) error {
	// Marshal to JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(f.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Generate timestamped filename: {caseID}_{timestamp}.json
	// Format: IOE0933798378_2025-10-11T15-04-05.json
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("%s_%s.json", f.caseID, timestamp)
	filePath := filepath.Join(f.stateDir, filename)

	// Write to temp file first for atomic write
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Rename temp file to actual file (atomic operation)
	if err := os.Rename(tempFile, filePath); err != nil {
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}

	return nil
}
