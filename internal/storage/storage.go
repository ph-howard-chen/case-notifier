package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Storage interface for persisting case status
type Storage interface {
	Load() (map[string]interface{}, error)
	Save(data map[string]interface{}) error
}

// FileStorage implements Storage using a JSON file
type FileStorage struct {
	filePath string
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		filePath: filePath,
	}
}

// Load loads the previous state from file
func (f *FileStorage) Load() (map[string]interface{}, error) {
	// Check if file exists
	if _, err := os.Stat(f.filePath); os.IsNotExist(err) {
		// File doesn't exist - first run
		return nil, nil
	}

	// Read file contents
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Parse JSON
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return state, nil
}

// Save saves the current state to file
func (f *FileStorage) Save(data map[string]interface{}) error {
	// Marshal to JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(f.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write to temp file first for atomic write
	tempFile := f.filePath + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Rename temp file to actual file (atomic operation)
	if err := os.Rename(tempFile, f.filePath); err != nil {
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}

	return nil
}
