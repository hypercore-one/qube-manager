# Qube Manager

A small Go CLI that listens to Nostr relays for upgrade or reboot votes from a configured follow list, tallies votes, picks the highest semantic version that meets quorum, skips anything already executed via a local history, then publishes a signed `done` message. You can also use it to publish your own vote.

---

## Features

- Connect to multiple Nostr relays and subscribe to kind=1 notes from a follow list
- Parse JSON message content for type: "upgrade" or "reboot"
- Validate version with SemVer, validate genesis as a URL for reboots
- Tally votes per action key
  - upgrade:<version>
  - reboot:<version>:<genesis>
- Filter to actions that meet quorum and are not in history
- Select the highest version among qualified actions
- Publish a signed done note to all configured relays unless --dry-run
- Persist decisions in history.yaml to stay idempotent
- send-message subcommand to publish your own vote
- Rotating file logs via lumberjack (manager.log in the config dir), plus stdout

---

## Install

Requires a current Go toolchain.

    git clone https://github.com/hypercore-one/qube-manager
    cd qube-manager
    go build -o qube-manager ./...

Cross-compile examples:

    GOOS=linux  GOARCH=amd64 go build -o qube-manager-linux-amd64  ./...
    GOOS=darwin GOARCH=arm64 go build -o qube-manager-darwin-arm64 ./...
    GOOS=windows GOARCH=amd64 go build -o qube-manager-windows-amd64.exe ./...

---

## Quick start

    # optional, use the default if you want
    mkdir -p ~/.qube-manager

    # first run without sending or writing history
    ./qube-manager --dry-run --verbose

On first run, the app ensures the config directory exists, loads or creates a keypair, loads config and history, connects to relays, listens for votes, and reports the decision it would take.

---

## CLI

    qube-manager [--dry-run] [--config-dir PATH] [--verbose]
    qube-manager send-message --type upgrade|reboot --version vX.Y.Z [--genesis URL] [--extra TEXT] [--dry-run]

Global flags

- --dry-run  
  Do not publish and do not write history.
- --config-dir PATH  
  Config directory. Default ~/.qube-manager.
- --verbose  
  More logs. Also surfaces go-nostr debug lines.

Subcommand: send-message

Publish a vote to your relays. The event is signed with your local nsec.

- --type  upgrade or reboot  
- --version  semantic version like v1.2.3  
- --genesis  URL string, required for reboot  
- --extra  optional string stored in extraData  
- --dry-run  print the JSON and exit without sending  

Examples:

    qube-manager send-message --type upgrade --version v1.4.0
    qube-manager send-message --type reboot  --version v2.0.0 --genesis https://example.org/genesis.json
    qube-manager send-message --type upgrade --version v1.4.0 --dry-run

---

## Configuration

config.go reads or creates ~/.qube-manager/config.yaml by default.

If the file does not exist, a default is written:

    relays:
      - wss://nostr.zenon.network
    follows:
      - npub1sr47j9awvw2xa0m4w770dr2rl7ylzq4xt9k5rel3h4h58sc3mjysx6pj64  # george
    quorum: 1

Rules and validation:

- follows must be NIP-19 npub strings. They are decoded and used as author filters.
- relays must be valid URLs.
- quorum is the minimum count of unique author pubkeys that voted for the same action key.

If you point --config-dir elsewhere, the same filenames are used inside that directory.

---

## Keys

keys.go manages ~/.qube-manager/keys.json. If missing or invalid, a new keypair is generated.

- Directory mode: 0700
- File mode: 0600

Structure:

    {
      "nsec": "nsec1...",
      "npub": "npub1..."
    }

Behavior:

- On startup the app reads keys.json. If it cannot parse it, it generates a new keypair using go-nostr, encodes it with NIP-19, and writes the file.
- The app decodes nsec at runtime and signs outgoing events.
- npub is used as the author of emitted notes.

Keep this file out of version control.

---

## History

history.go manages ~/.qube-manager/history.yaml to prevent repeats.

- File mode when writing: 0644
- Format:

    entries:
      upgrade:v1.2.3: "2025-11-07T14:33:12Z"
      reboot:v2.0.0:https://example.org/genesis.json: "2025-11-07T15:00:02Z"

Methods:

- Has(key) to check if an action was already executed
- Add(key) to record the current UTC timestamp
- Save() to persist

Action keys:

- upgrade:<version>
- reboot:<version>:<genesis>

---

## Message formats

Incoming votes must be JSON in the Nostr event content.

Upgrade vote

    {"type":"upgrade","version":"v1.2.3"}

Reboot vote

    {"type":"reboot","version":"v2.0.0","genesis":"https://example.com/genesis.json"}

Done messages published by the tool after execution:

Upgrade:

    {"type":"upgrade","version":"v1.2.3","extraData":"done"}

Reboot:

    {"type":"reboot","version":"v2.0.0","genesis":"https://example.com/genesis.json","extraData":"done"}

---

## Decision rules

- Only kind=1 notes from decoded authors in follows are considered.
- version must parse as a semantic version.
- Reboot votes must include a valid URL string in genesis.
- Votes are counted once per unique author pubkey for the same action key.
- Actions with votes below quorum are ignored.
- Among qualified actions, the highest SemVer is selected.
- Already executed action keys are skipped based on history.yaml.

---

## Logging

Logging is configured by setupLogging(configDir) and configureNostrLogging(verbose).

- Output targets: stdout and a rotating file at ~/.qube-manager/manager.log (or config-dir/manager.log)
- Rotation via gopkg.in/natefinch/lumberjack.v2
  - MaxSize: 10 MB
  - MaxBackups: 3
  - MaxAge: 28 days
  - Compress: true
- Log format: standard flags with short file (log.LstdFlags | log.Lshortfile)
- Nostr logs: if --verbose is false, go-nostr InfoLogger is silenced (sent to io.Discard)

Timeouts:

- Relay connect uses a 10 second context to avoid hangs.
- Publishing and send-message use bounded contexts.

---

## Dependencies

- github.com/nbd-wtf/go-nostr and github.com/nbd-wtf/go-nostr/nip19
- github.com/Masterminds/semver/v3
- gopkg.in/yaml.v3
- gopkg.in/natefinch/lumberjack.v2

---

## Security notes

- Your nsec signs everything. Protect ~/.qube-manager and keys.json.
- history.yaml is written with 0644. Tighten this if your environment requires it.
- genesis is validated as a URL string only. Validate content and provenance before acting on it downstream.

---

## Development

    go build -o qube-manager ./...
    go test ./...
    go vet ./...

---

## License

MIT. See LICENSE.

