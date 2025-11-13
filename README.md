# Qube Manager

A Go CLI that listens to Nostr relays for custom event kinds that dictate HyperQube directives and acknowledgements. It keeps track of events from a configured follow list, tallies votes, picks the latest qualified action that meets quorum, and publishes signed ACK events to all configured relays.

---

## Features

- Connect to multiple Nostr relays and subscribe to listen for HyperSignal events (event kind 33321).
- Parse structured event tags for action type, version, network, and metadata
- Validate semantic versions and genesis URLs for reboot actions
- Tally votes per action key using quorum-based decision making
- Filter actions that meet quorum and are not in execution history
- Select highest semantic version among qualified actions
- Publish signed ACK events to all configured relays (unless --dry-run)
- Persist decisions in history.yaml to ensure idempotency
- Rotating file logs via lumberjack (manager.log in config dir), plus stdout

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

On first run, the app ensures the config directory exists, loads or creates a keypair, loads config and history, connects to relays, listens for HyperSignal events, and reports the decision it would take.

---

## CLI

    qube-manager [--dry-run] [--config-dir PATH] [--verbose]

Global flags

- --dry-run  
  Do not publish and do not write history.
- --config-dir PATH  
  Config directory. Default ~/.qube-manager.
- --verbose  
  More logs. Also surfaces go-nostr debug lines.


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
- quorum is the minimum number of trusted developers required to publish identical HyperSignal directives before nodes will execute the specified action.

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

- On startup, the app reads keys.json. If it cannot parse it, it generates a new keypair using go-nostr, encodes it with NIP-19, and writes the file.
- The app decodes nsec at runtime and signs outgoing events.
- npub is used as the author of emitted events.

Keep this file out of version control.

---

## History

history.go manages ~/.qube-manager/history.yaml to prevent repeats.

- File mode when writing: 0644
- Format:

    entries:
      "33321:developer_pubkey:hyperqube": "2025-11-12T14:33:12Z"
      "33321:another_dev_pubkey:hyperqube": "2025-11-12T15:00:02Z"

Methods:

- Has(key) to check if a directive was already executed
- Add(key) to record current UTC timestamp
- Save() to persist

Directive keys:

- 33321:<developer_pubkey>:hyperqube (addressable event identifier)

---

## Event Formats

### Kind 33321: HyperSignal Events

HyperSignal events are addressable (parameterized replaceable) events that contain directives in structured tags.

Example upgrade directive:

```json
{
  "kind": 33321,
  "pubkey": "<developer_pubkey>",
  "created_at": 1703980800,
  "tags": [
    ["d", "hyperqube"],
    ["version", "v1.4.0"],
    ["hash", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"],
    ["network", "hqz"],
    ["action", "upgrade"]
  ],
  "content": "[hypersignal] A HyperQube upgrade has been released. Please update binary to version v1.4.0.",
  "sig": "<64-byte signature>"
}
```

Example reboot directive:

```json
{
  "kind": 33321,
  "pubkey": "<developer_pubkey>",
  "created_at": 1703980800,
  "tags": [
    ["d", "hyperqube"],
    ["version", "v2.0.0"],
    ["network", "hqz"],
    ["action", "reboot"],
    ["genesis_url", "https://example.org/genesis.json"],
    ["required_by", "1704067200"]
  ],
  "content": "[hypersignal] A HyperQube reboot for version v2.0.0 has been scheduled. Please reboot by 2024-01-01T00:00:00Z.",
  "sig": "<64-byte signature>"
}
```

### Kind 3333: ACK Events

ACK events are regular events that acknowledge the completion of HyperSignal directives.

Example successful upgrade acknowledgement:

```json
{
  "kind": 3333,
  "pubkey": "<node_pubkey>",
  "created_at": 1703984400,
  "tags": [
    ["a", "33321:<developer_pubkey>:hyperqube", "wss://nostr.zenon.network"],
    ["p", "<developer_pubkey>", "wss://nostr.zenon.network"],
    ["version", "v1.4.0"],
    ["network", "hqz"],
    ["action", "upgrade"],
    ["status", "success"],
    ["node_id", "hyperqube-node-xyz789"],
    ["action_at", "1703984300"]
  ],
  "content": "[qube-manager] The upgrade to version v1.4.0 has been successful on hyperqube-node-xyz789.",
  "sig": "<64-byte signature>"
}
```

---

## Decision Rules

- Only Kind 33321 events from decoded authors in follows are considered.
- Events must have `["d", "hyperqube"]` tag to be valid HyperSignal events.
- version must parse as a semantic version.
- Reboot actions must include a valid URL string in genesis_url tag.
- Votes are limited to one per unique kind:pubkey:d identifier for each action.
- Actions with votes below quorum are ignored.
- Among qualified actions, the highest SemVer is selected.
- Directive keys already recorded in history.yaml are skipped.

---

## Event Tag Specifications

### HyperSignal Event Tags (Kind 33321)

| Tag | Required | Description | Example |
|-----|----------|-------------|---------|
| `d` | Yes | Address identifier, always "hyperqube" | `["d", "hyperqube"]` |
| `version` | Yes | Target semantic version | `["version", "v1.4.0"]` |
| `hash` | Yes | SHA256 hash of binary/artifact | `["hash", "a1b2c3d4..."]` |
| `network` | Yes | Target network identifier | `["network", "hqz"]` |
| `action` | Yes | Action type: "upgrade" or "reboot" | `["action", "upgrade"]` |
| `genesis_url` | For reboot | URL for genesis data | `["genesis_url", "https://example.com/genesis.json"]` |
| `required_by` | For reboot | Deadline timestamp | `["required_by", "1704067200"]` |

### ACK Event Tags (Kind 3333)

| Tag | Required | Description | Example |
|-----|----------|-------------|---------|
| `a` | Yes | Reference to HyperSignal event | `["a", "33321:<pubkey>:hyperqube", "relay_url"]` |
| `p` | Recommended | Author of original directive | `["p", "<developer_pubkey>", "relay_url"]` |
| `version` | Yes | Version of action performed | `["version", "v1.4.0"]` |
| `network` | Yes | Network identifier | `["network", "hqz"]` |
| `action` | Yes | Action type performed | `["action", "upgrade"]` |
| `status` | Yes | Outcome: "success" or "failure" | `["status", "success"]` |
| `node_id` | Yes | Unique node identifier | `["node_id", "hyperqube-node-xyz789"]` |
| `action_at` | Yes | Timestamp of action completion | `["action_at", "1703984300"]` |
| `error` | Optional | Error message if status is "failure" | `["error", "hash mismatch"]` |

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
- Event publishing and processing use bounded contexts.

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
- genesis_url is validated as a URL string only. Validate content and provenance before acting on it downstream.

---

## Development

    go build -o qube-manager ./...
    go test ./...
    go vet ./...

---

## License

MIT. See LICENSE.