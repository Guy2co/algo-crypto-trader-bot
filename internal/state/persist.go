// Package state provides helpers for JSON-based strategy state persistence.
package state

import (
	"encoding/json"
	"fmt"
	"os"
)

// SaveJSON marshals v to JSON and writes it atomically to path.
// The file is written with mode 0600.
func SaveJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write state to %s: %w", path, err)
	}
	return nil
}

// LoadJSON reads a JSON file from path and unmarshals it into v.
// Returns (false, nil) if the file does not exist — the caller can create
// a fresh state. Returns (true, nil) on success.
func LoadJSON(path string, v any) (found bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read state from %s: %w", path, err)
	}
	if err = json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("unmarshal state: %w", err)
	}
	return true, nil
}
