package main

import (
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/nbd-wtf/go-nostr/nip19"
	"gopkg.in/yaml.v3"
)

// Config holds application settings loaded from YAML config file
type Config struct {
	Relays     []string `yaml:"relays"`  // List of relay URLs to connect to
	Follows    []string `yaml:"follows"` // List of Nostr npubs to follow
	Quorum     int      `yaml:"quorum"`  // Number of follows needed to trigger action
	ConfigPath string   `yaml:"-"`       // Path to config directory (not in YAML)
}

// loadConfig reads the YAML config file or creates a default one if missing,
// then validates npubs and relay URLs.
func loadConfig(configDir string) Config {
	path := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("[WARN] Config file not found at %s, creating default config", path)
		defaultCfg := Config{
			Relays: []string{"wss://nostr.zenon.network"},
			Follows: []string{
				"npub1sr47j9awvw2xa0m4w770dr2rl7ylzq4xt9k5rel3h4h58sc3mjysx6pj64", // george
			},
			Quorum: 1,
		}
		data, err := yaml.Marshal(defaultCfg)
		if err != nil {
			log.Fatalf("[ERROR] Failed to marshal default config: %v", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			log.Fatalf("[ERROR] Failed to write default config to %s: %v", path, err)
		}
		log.Printf("[INFO] Default config created at %s", path)
	} else if err != nil {
		log.Fatalf("[ERROR] Error checking config file %s: %v", path, err)
	} else {
		log.Printf("[INFO] Config file found at %s, loading", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("[ERROR] Failed to read config file %s: %v", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("[ERROR] Failed to parse config file %s: %v", path, err)
	}
	cfg.ConfigPath = configDir
	log.Printf("[INFO] Loaded config: %d relay(s), %d follow(s), quorum=%d", len(cfg.Relays), len(cfg.Follows), cfg.Quorum)

	// Validate npubs
	for _, npub := range cfg.Follows {
		kind, _, err := nip19.Decode(npub)
		if err != nil {
			log.Fatalf("[ERROR] Invalid npub in config: %v", err)
		}
		if kind != "npub" {
			log.Fatalf("[ERROR] Expected npub but got %s in config: %s", kind, npub)
		}
	}

	// Validate relay URLs
	for _, r := range cfg.Relays {
		if _, err := url.ParseRequestURI(r); err != nil {
			log.Fatalf("[ERROR] Invalid relay URL in config: %s", r)
		}
	}

	return cfg
}
