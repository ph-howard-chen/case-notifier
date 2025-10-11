package uscis

import (
	"fmt"
	"reflect"
	"strings"
)

// Change represents a single field change
type Change struct {
	Field    string
	OldValue interface{}
	NewValue interface{}
}

// DetectChanges compares two case status maps and returns a list of changes
func DetectChanges(previous, current map[string]interface{}) []Change {
	if previous == nil {
		// First run - no previous state
		return nil
	}

	var changes []Change

	// Check for changed or new fields in current
	for key, newVal := range current {
		oldVal, exists := previous[key]

		if !exists {
			// New field added
			changes = append(changes, Change{
				Field:    key,
				OldValue: nil,
				NewValue: newVal,
			})
		} else if !deepEqual(oldVal, newVal) {
			// Field value changed
			changes = append(changes, Change{
				Field:    key,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	// Check for removed fields
	for key, oldVal := range previous {
		if _, exists := current[key]; !exists {
			changes = append(changes, Change{
				Field:    key,
				OldValue: oldVal,
				NewValue: nil,
			})
		}
	}

	return changes
}

// deepEqual performs deep comparison of two values
func deepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// FormatChanges formats a list of changes into a human-readable string
func FormatChanges(changes []Change) string {
	if len(changes) == 0 {
		return "No changes detected"
	}

	var lines []string
	for _, change := range changes {
		lines = append(lines, formatChange(change))
	}

	return strings.Join(lines, "\n")
}

// formatChange formats a single change into a readable string
func formatChange(change Change) string {
	if change.OldValue == nil {
		return fmt.Sprintf("+ %s: %v (new)", change.Field, change.NewValue)
	} else if change.NewValue == nil {
		return fmt.Sprintf("- %s: %v (removed)", change.Field, change.OldValue)
	} else {
		return fmt.Sprintf("~ %s: %v → %v", change.Field, change.OldValue, change.NewValue)
	}
}
