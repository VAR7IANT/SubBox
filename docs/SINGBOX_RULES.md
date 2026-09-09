# Sing-box Rules

This document defines behavior that Luna must not reinterpret during implementation.

## Supported Protocols

Phase 1:

- Shadowsocks
- Hysteria2
- TUIC
- VLESS Reality
- AnyTLS

## Port Semantics

Every node has two separate port concepts.

### `listen_port`

The local server port placed in Sing-box's inbound configuration.

### `public_port`

The client-facing port after NAT / port forwarding. This port is written into generated client URIs and subscription output.

Example:

```text
Internet client -> example.com:24443
                         |
                       NAT
                         |
                  local server:443
```

Stored as:

```text
listen_port = 443
public_port = 24443
```

Changing `public_port` alone must not rewrite Sing-box's inbound `listen_port`.

Changing `listen_port` requires a safe Sing-box config transaction.

## Refresh

Subscription Refresh means:

1. read current enabled nodes from persisted state
2. generate fresh subscription content
3. update content timestamp/cache if used
4. keep the subscription token/URL unchanged

Refresh MUST NOT regenerate:

- UUID
- protocol password
- Reality key pair
- Reality Short ID
- other authentication credentials

Refresh should not invalidate currently working clients.

## Rotate

Rotate is a separate explicit operation.

General flow:

1. generate protocol-specific replacement credentials
2. build candidate Sing-box config
3. apply via config transaction
4. verify Sing-box service
5. persist new credential state
6. regenerate subscription content

Examples of credential rotation targets:

- Shadowsocks: password
- Hysteria2: password
- TUIC: UUID + password
- VLESS Reality: UUID and Reality credential material as defined by final protocol model
- AnyTLS: AnyTLS credential material and Reality material when configured that way

Final protocol-specific rotation behavior must be reviewed by Sol before implementation.

## Config Transaction

All changes that affect the live Sing-box server configuration use one shared transaction path.

Required conceptual flow:

```text
Validate Request
      |
Acquire Config Lock
      |
Read Current Working Config
      |
Create Backup
      |
Build Candidate Config
      |
Write Candidate Temp File
      |
sing-box check -c <candidate>
      |
Atomic Replace Live Config
      |
Restart / Reload sing-box
      |
Verify Service
      |
Commit Application State
```

If any step after mutation begins fails:

```text
Restore Previous Config
      |
Restart / Restore Service
      |
Verify Previous Working State
```

## Concurrency

Only one config mutation transaction may run at a time.

Concurrent requests that would modify the Sing-box config must serialize or fail cleanly.

## File Safety

Use fixed trusted paths.

Expected paths:

- live config: `/etc/sing-box/config.json`
- SubBox state directory: `/etc/subbox/`
- backups: a fixed directory under `/etc/subbox/`

Candidate files should be written safely and replaced atomically where supported.

## Validation

A candidate configuration must pass `sing-box check` before replacing the live config.

A successful `sing-box check` alone is not enough: service restart/reload must also be verified.

## URI Generation

Server configuration uses `listen_port`.

Client URI/subscription generation uses:

- public host
- `public_port`
- protocol credentials
- protocol-specific parameters

URI generation must not accidentally expose server-only private keys.

## Service Management

Phase 1 service actions:

- status
- start
- stop
- restart

Support Systemd and OpenRC behind an internal abstraction.

Do not expose a generic arbitrary-service manager.

## Original Reference Repository

`caigouzi121380/singbox-deploy` may be consulted to understand deployment behavior, OS differences, supported protocol configuration, and URI patterns.

SubBox should implement its own structured Go modules rather than automating interactive `read -p` shell flows.
