package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// UpgradeMessage represents the "upgrade" message type
type UpgradeMessage struct {
	Type      string `json:"type"`                // Must be "upgrade"
	Version   string `json:"version"`             // Semantic version string
	ExtraData string `json:"extraData,omitempty"` // additional metadata or status
}

// RebootMessage represents the "reboot" message type
type RebootMessage struct {
	Type      string `json:"type"`                // Must be "reboot"
	Version   string `json:"version"`             // Semantic version string
	Genesis   string `json:"genesis"`             // URL string
	ExtraData string `json:"extraData,omitempty"` // additional metadata or status
}

func sendMessageCLI(configDir string) {
	var (
		msgType string
		version string
		genesis string
		extra   string
		dryRun  bool
	)

	flagSet := flag.NewFlagSet("send-message", flag.ExitOnError)
	flagSet.StringVar(&msgType, "type", "", "Message type: 'upgrade' or 'reboot'")
	flagSet.StringVar(&version, "version", "", "Semantic version (e.g. v1.2.3)")
	flagSet.StringVar(&genesis, "genesis", "", "Genesis URL (required for 'reboot')")
	flagSet.StringVar(&extra, "extra", "", "Extra data (optional)")
	flagSet.BoolVar(&dryRun, "dry-run", false, "Print message instead of sending")
	flagSet.Parse(os.Args[2:])

	// Validate message type
	if msgType != "upgrade" && msgType != "reboot" {
		log.Fatalf("[ERROR] Invalid message type '%s'. Must be 'upgrade' or 'reboot'.", msgType)
	}

	// Validate version
	if version == "" {
		log.Fatal("[ERROR] Version is required.")
	}
	if _, err := semver.NewVersion(version); err != nil {
		log.Fatalf("[ERROR] Invalid semantic version '%s': %v", version, err)
	}

	// Validate genesis for reboot
	if msgType == "reboot" && genesis == "" {
		log.Fatal("[ERROR] Genesis URL is required for reboot messages.")
	}

	// Build message content
	var content []byte
	var err error
	switch msgType {
	case "upgrade":
		content, err = json.Marshal(UpgradeMessage{
			Type:      "upgrade",
			Version:   version,
			ExtraData: extra,
		})
	case "reboot":
		content, err = json.Marshal(RebootMessage{
			Type:      "reboot",
			Version:   version,
			Genesis:   genesis,
			ExtraData: extra,
		})
	}
	if err != nil {
		log.Fatalf("[ERROR] Failed to marshal message: %v", err)
	}

	if dryRun {
		log.Println("[DRY RUN] Prepared message to publish:")
		fmt.Println(string(content))
		return
	}

	log.Printf("[INFO] Loading keypair from config directory: %s", configDir)
	kp := loadOrCreateKeypair(configDir)
	_, privKey, err := nip19.Decode(kp.Nsec)
	if err != nil {
		log.Fatalf("[ERROR] Invalid private key: %v", err)
	}

	cfg := loadConfig(configDir)
	if len(cfg.Relays) == 0 {
		log.Println("[WARN] No relays configured; message will not be sent.")
		return
	}

	ev := nostr.Event{
		PubKey:    kp.Npub,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      nostr.KindTextNote,
		Content:   string(content),
	}
	if err := ev.Sign(privKey.(string)); err != nil {
		log.Fatalf("[ERROR] Failed to sign event: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, relayURL := range cfg.Relays {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			log.Printf("[INFO] Connecting to relay %s", url)
			r, err := nostr.RelayConnect(ctx, url)
			if err != nil {
				log.Printf("[WARN] Could not connect to relay %s: %v", url, err)
				return
			}
			defer r.Close()

			log.Printf("[INFO] Publishing message to relay %s", url)
			if err := r.Publish(ctx, ev); err != nil {
				log.Printf("[WARN] Failed to publish to relay %s: %v", url, err)
				return
			}

			log.Printf("[INFO] Successfully published message to relay %s", url)
		}(relayURL)
	}

	wg.Wait()
	log.Println("[INFO] Finished publishing message to all configured relays.")
}
