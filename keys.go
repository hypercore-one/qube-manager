package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

type Keypair struct {
	Nsec string `json:"nsec"` // nsec...
	Npub string `json:"npub"` // npub...
}

func loadOrCreateKeypair(configDir string) Keypair {
	keyPath := filepath.Join(configDir, "keys.json")

	if data, err := os.ReadFile(keyPath); err == nil {
		var kp Keypair
		if json.Unmarshal(data, &kp) == nil {
			return kp
		}
	}

	// Generate new key
	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)
	nsec, _ := nip19.EncodePrivateKey(sk)
	npub, _ := nip19.EncodePublicKey(pk)

	kp := Keypair{
		Nsec: nsec,
		Npub: npub,
	}
	data, _ := json.MarshalIndent(kp, "", "  ")
	os.MkdirAll(configDir, 0700)
	_ = os.WriteFile(keyPath, data, 0600)
	return kp
}
