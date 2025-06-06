package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// CandidateAction holds details of a potential action to perform
type CandidateAction struct {
	Version *semver.Version // Parsed semantic version
	Type    string          // "upgrade" or "reboot"
	Key     string          // Unique history key
	Genesis string          // Genesis URL for reboot, empty for upgrade
}

func main() {
	// Command-line flags
	var (
		dryRun    = flag.Bool("dry-run", false, "Perform a trial run without saving actions")
		configDir = flag.String("config-dir", filepath.Join(os.Getenv("HOME"), ".qube-manager"), "Configuration directory")
		verbose   = flag.Bool("verbose", false, "Enable verbose logging including go-nostr logs")
	)
	flag.Parse()

	log.Printf("[INFO] Starting Qube Manager")

	if err := os.MkdirAll(*configDir, 0755); err != nil {
		log.Fatalf("[ERROR] Failed to create config directory: %v", err)
	} else {
		log.Printf("[INFO] Ensured config directory exists at %s", *configDir)
	}

	// Setup logging to file and stdout
	setupLogging(*configDir)

	if *dryRun {
		log.Println("[INFO] Running in dry-run mode")
	}
	if *verbose {
		log.Println("[INFO] Verbose logging enabled")
	}

	log.Println("[INFO] Loading or creating keypair")
	keypair := loadOrCreateKeypair(*configDir)
	_, _, err := nip19.Decode(keypair.Nsec)
	if err != nil {
		log.Fatalf("[ERROR] Invalid private key in config: %v", err)
	}

	// Suppress go-nostr info logs like "filter doesn't match"
	configureNostrLogging(*verbose)
	log.Println("[INFO] Nostr logging configured")

	if len(os.Args) > 1 && os.Args[1] == "send-message" {
		log.Println("[INFO] Handling 'send-message' command")
		sendMessageCLI(*configDir)
		return
	}

	// Load configuration and history from files
	config := loadConfig(*configDir)
	history := loadHistory(*configDir)

	log.Printf("[INFO] Loaded config: %d relays, %d follows, quorum=%d",
		len(config.Relays), len(config.Follows), config.Quorum)

	// Context with timeout to avoid hanging connections
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Map to hold candidate actions keyed by unique history keys
	actions := make(map[string]*CandidateAction)

	// Map of action key -> set of pubkeys that voted for this action
	votes := make(map[string]map[string]bool)

	// Connect to each relay and subscribe to relevant events
	for _, relayURL := range config.Relays {
		start := time.Now()
		log.Printf("[INFO] Connecting to relay: %s", relayURL)
		relay, err := nostr.RelayConnect(ctx, relayURL)
		if err != nil {
			log.Printf("[WARN] Failed to connect to relay %s: %v (took %v)", relayURL, err, time.Since(start))
			continue
		}
		log.Printf("[INFO] Connected to relay: %s (took %v)", relayURL, time.Since(start))

		// Decode all npubs to hex pubkeys for filtering
		hexFollows := make([]string, 0, len(config.Follows))
		for _, npub := range config.Follows {
			kind, pubkeyAny, err := nip19.Decode(npub)
			if err != nil {
				log.Printf("[WARN] Skipping invalid npub (%s): %v", npub, err)
				continue
			}
			if kind != "npub" {
				log.Printf("[WARN] Expected npub but got %s: %s", kind, npub)
				continue
			}
			pubkey, ok := pubkeyAny.(string)
			if !ok {
				log.Printf("[WARN] Unexpected pubkey format for %s: %v", npub, pubkeyAny)
				continue
			}
			hexFollows = append(hexFollows, pubkey)
		}
		log.Printf("[INFO] Relay %s: decoded %d valid npubs for following", relayURL, len(hexFollows))

		// Subscribe to kind=1 events authored by followed pubkeys
		sub, err := relay.Subscribe(ctx, nostr.Filters{{
			Authors: hexFollows,
			Kinds:   []int{1},
		}})
		if err != nil {
			log.Printf("[ERROR] Subscription failed on %s: %v", relayURL, err)
			continue
		}
		log.Printf("[INFO] Subscription successful on %s", relayURL)

		// Ensure subscription gets cleaned up
		defer func(relayURL string) {
			log.Printf("[INFO] Closing subscription on %s", relayURL)
			sub.Close()
			log.Printf("[INFO] Subscription on relay %s closed", relayURL)
		}(relayURL)

		// Read events and parse messages
		for ev := range sub.Events {
			// Try to detect message type early
			var meta struct{ Type string }
			if err := json.Unmarshal([]byte(ev.Content), &meta); err != nil {
				if *verbose {
					log.Printf("[DEBUG] Skipping event with invalid JSON from pubkey %s: %s", ev.PubKey, ev.Content)
				}
				continue
			}

			switch meta.Type {
			case "upgrade":
				var msg UpgradeMessage
				if err := json.Unmarshal([]byte(ev.Content), &msg); err != nil {
					log.Printf("[WARN] Failed to parse upgrade message: %v", err)
					continue
				}

				v, err := semver.NewVersion(msg.Version)
				if err != nil {
					log.Printf("[WARN] Invalid semantic version in upgrade: %s", msg.Version)
					continue
				}

				key := fmt.Sprintf("upgrade:%s", v.Original())
				action, exists := actions[key]
				if !exists {
					action = &CandidateAction{
						Type:    "upgrade",
						Version: v,
						Key:     key,
					}
					actions[key] = action
				}

				if votes[key] == nil {
					votes[key] = make(map[string]bool)
				}
				votes[key][ev.PubKey] = true

				log.Printf("[INFO] Parsed upgrade message: version=%s pubkey=%s", v.Original(), ev.PubKey)

			case "reboot":
				var msg RebootMessage
				if err := json.Unmarshal([]byte(ev.Content), &msg); err != nil {
					log.Printf("[WARN] Failed to parse reboot message: %v", err)
					continue
				}

				if _, err := url.ParseRequestURI(msg.Genesis); err != nil {
					log.Printf("[WARN] Invalid genesis URL in reboot: %s", msg.Genesis)
					continue
				}

				v, err := semver.NewVersion(msg.Version)
				if err != nil {
					log.Printf("[WARN] Invalid semantic version in reboot: %s", msg.Version)
					continue
				}

				key := fmt.Sprintf("reboot:%s:%s", v.Original(), msg.Genesis)
				action, exists := actions[key]
				if !exists {
					action = &CandidateAction{
						Type:    "reboot",
						Version: v,
						Key:     key,
						Genesis: msg.Genesis,
					}
					actions[key] = action
				}

				if votes[key] == nil {
					votes[key] = make(map[string]bool)
				}
				votes[key][ev.PubKey] = true

				log.Printf("[INFO] Parsed reboot message: version=%s genesis=%s pubkey=%s", v.Original(), msg.Genesis, ev.PubKey)

			default:
				if *verbose {
					log.Printf("[DEBUG] Ignoring event with unknown type: %s", meta.Type)
				}
			}
		}
	}

	// Select the latest semver action meeting quorum and not already in history
	var latest *CandidateAction
	for _, a := range actions {
		if history.Has(a.Key) {
			continue // skip already acted on
		}

		voteCount := 0
		if vset, ok := votes[a.Key]; ok {
			voteCount = len(vset)
		}

		if voteCount < config.Quorum {
			log.Printf("[INFO] Skipping action %s - votes %d/%d (below quorum)", a.Key, voteCount, config.Quorum)
			continue
		}

		if latest == nil || a.Version.GreaterThan(latest.Version) {
			latest = a
		}
	}

	if latest != nil {
		log.Printf("[INFO] Selected action %s with version %s and %d votes",
			latest.Key, latest.Version.Original(), len(votes[latest.Key]))

		switch latest.Type {
		case "upgrade":
			log.Printf("[UPGRADE ACTION] Version: %s", latest.Version.Original())
		case "reboot":
			log.Printf("[REBOOT ACTION] Version: %s Genesis: %s", latest.Version.Original(), latest.Genesis)
		}

		if !*dryRun {
			var content []byte
			var err error

			switch latest.Type {
			case "upgrade":
				doneMsg := UpgradeMessage{
					Type:      "upgrade",
					Version:   latest.Version.Original(),
					ExtraData: "done",
				}
				content, err = json.Marshal(doneMsg)

			case "reboot":
				doneMsg := RebootMessage{
					Type:      "reboot",
					Version:   latest.Version.Original(),
					Genesis:   latest.Genesis,
					ExtraData: "done",
				}
				content, err = json.Marshal(doneMsg)
			}

			if err != nil {
				log.Printf("[ERROR] Failed to marshal done message: %v", err)
				return
			}

			doneEvent := nostr.Event{
				PubKey:    keypair.Npub,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindTextNote,
				Content:   string(content),
			}

			_, priv, err := nip19.Decode(keypair.Nsec)
			if err != nil {
				log.Fatalf("[ERROR] Invalid private key: %v", err)
			}

			if err := doneEvent.Sign(priv.(string)); err != nil {
				log.Printf("[ERROR] Error signing done event: %v", err)
				return
			}

			log.Printf("[INFO] Publishing done event for action %s to %d relays", latest.Key, len(config.Relays))

			for _, r := range config.Relays {
				go func(url string) {
					log.Printf("[INFO] Publishing to relay %s", url)
					if relay, err := nostr.RelayConnect(context.Background(), url); err == nil {
						_ = relay.Publish(context.Background(), doneEvent)
					} else {
						log.Printf("[WARN] Relay publish error (%s): %v", url, err)
					}
				}(r)
			}

			history.Add(latest.Key)
			if err := history.Save(); err != nil {
				log.Printf("[WARN] Error saving history: %v", err)
			} else {
				log.Printf("[INFO] Action %s saved to history", latest.Key)
			}
		} else {
			log.Println("[INFO] Dry run - not saving action to history.")
		}
	} else {
		log.Println("[INFO] No new eligible actions to perform.")
	}
}
