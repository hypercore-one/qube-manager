package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// History tracks performed actions to ensure idempotency
type History struct {
	Entries map[string]string `yaml:"entries"` // key: message key, value: ISO8601 timestamp
	path    string            // history file path (not in YAML)
}

// Has checks if an action key is already recorded in history
func (h *History) Has(key string) bool {
	_, ok := h.Entries[key]
	return ok
}

// Add records a new action with the current UTC timestamp
func (h *History) Add(key string) {
	h.Entries[key] = time.Now().UTC().Format(time.RFC3339)
	log.Printf("[INFO] Added history entry for key: %s", key)
}

// Save writes the history back to the YAML file
func (h *History) Save() error {
	data, err := yaml.Marshal(h)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal history: %v", err)
		return err
	}
	if err := os.WriteFile(h.path, data, 0644); err != nil {
		log.Printf("[ERROR] Failed to write history file %s: %v", h.path, err)
		return err
	}
	log.Printf("[INFO] History saved successfully to %s", h.path)
	return nil
}

// loadHistory reads the YAML history file or creates a new empty history if missing
func loadHistory(configDir string) *History {
	path := filepath.Join(configDir, "history.yaml")
	h := &History{
		Entries: make(map[string]string),
		path:    path,
	}

	if _, err := os.Stat(path); err == nil {
		log.Printf("[INFO] Loading existing history file from %s", path)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("[ERROR] Failed to read history file %s: %v", path, err)
		}
		if err := yaml.Unmarshal(data, h); err != nil {
			log.Fatalf("[ERROR] Failed to parse history file %s: %v", path, err)
		}
		log.Printf("[INFO] History loaded: %d entries", len(h.Entries))
	} else if os.IsNotExist(err) {
		log.Printf("[WARN] History file does not exist, creating new one at %s", path)
		if err := h.Save(); err != nil {
			log.Fatalf("[ERROR] Failed to create history file %s: %v", path, err)
		}
	} else {
		log.Fatalf("[ERROR] Error checking history file %s: %v", path, err)
	}

	return h
}
