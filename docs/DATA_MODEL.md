# Data Model

This is the initial data model. Sol should finalize migrations and credential storage details before backend implementation.

## Node

Suggested fields:

```text
id              string / UUID
name            string
protocol        enum
host            string
listen_port     integer
public_port     integer
enabled         boolean
credentials     protocol-specific structured data
created_at      timestamp
updated_at      timestamp
```

### Protocol enum

Initial values:

- `shadowsocks`
- `hysteria2`
- `tuic`
- `vless_reality`
- `anytls`

### Port semantics

`listen_port`:

The local port used by Sing-box in `/etc/sing-box/config.json`.

`public_port`:

The port written into client-facing URIs and subscriptions after NAT / port forwarding.

Never replace these two fields with one generic `port` column.

## Subscription

Suggested fields:

```text
id                  string / UUID
token               high-entropy opaque string
enabled             boolean
content_updated_at  timestamp
created_at          timestamp
updated_at          timestamp
```

Rules:

- token remains stable across node edits
- token remains stable across port changes
- token remains stable across Refresh
- token remains stable across Rotate
- explicit token regeneration/revocation can be designed separately

## Settings

Suggested initial fields/concepts:

```text
id / singleton key
public_base_url
ui_language
created_at
updated_at
```

Do not store monitoring-agent configuration because SubBox has no monitoring agent.

## Credentials

Protocol-specific credential structures should be typed in Go rather than treated as arbitrary unvalidated JSON wherever practical.

Examples:

### Shadowsocks

- method
- password

### Hysteria2

- password
- TLS-related settings required by the server config

### TUIC

- uuid
- password

### VLESS Reality

- uuid
- server_name / SNI
- private key
- public key
- short ID

### AnyTLS

- username/name when required
- password
- Reality-related fields when used

Credential storage requires a security review before release. Secrets must never be included in ordinary application logs.

## Persistence Rule

Application state should be committed only after a Sing-box configuration transaction succeeds where the state change affects the live proxy configuration.
