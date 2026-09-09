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

Changing `public_port` alone must not invoke `sing-box check`, replace the live
config, or restart/reload the service. It is a SQLite + subscription snapshot
transaction only.

Changing `listen_port` requires a safe Sing-box config transaction.

Phase 1 does not provision external NAT/firewall/provider mappings.
`public_port` records the operator-managed client-facing mapping only. Protocol
adapters derive TCP/UDP listener behavior; enabled inbounds must be checked for
local bind conflicts before `sing-box check`.

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

Steps 5 and 6 are one SQLite transaction. The config mutation lock remains
held through that commit. If it fails, restore the old live config and prior
service state; new credentials must not remain active while old credentials are
still published.

Phase 1 Rotate is node-scoped. There is no implicit “rotate every node” action.
The stable subscription token and URL do not change.

Examples of credential rotation targets:

- Shadowsocks: password only; keep method/network unchanged
- Hysteria2: user authentication password only; keep TLS certificate/key,
  obfuscation password, and transport settings unchanged
- TUIC: user UUID and password; keep TLS certificate/key and transport settings
  unchanged
- VLESS Reality: user UUID, Reality key pair, and configured Short IDs; keep
  handshake target/server name, flow, and transport settings unchanged
- AnyTLS: user password only; keep user display name, TLS certificate/key, and
  padding settings unchanged

These are the locked Phase 1 targets. A future “rotate TLS certificate”,
“rotate obfuscation secret”, or “rotate all nodes” operation requires its own
contract and confirmation flow.

## Config Transaction

All changes that affect the live Sing-box server configuration use one shared transaction path.

Required conceptual flow:

```text
Validate Request
      |
Acquire Config Lock
      |
Recover/Reject Incomplete Transaction
      |
Snapshot Current Config, DB Revision, and Service State
      |
Build Candidate Config
      |
Write + fsync Candidate Temp File
      |
sing-box check -c <candidate>
      |
Create Durable Backup + Transaction Marker
      |
Atomic Replace Live Config + fsync Directory
      |
Restore Intended Service State + Verify
      |
Commit Application State + Subscription Snapshot
      |
Remove Marker + Cleanup
```

The service state before the transaction is authoritative:

- if it was running, restart/reload and verify it is running with the candidate
- if it was stopped, do not unexpectedly start it; verify the candidate with
  `sing-box check` and leave it stopped

If any step after the live file changes fails, including database commit:

```text
Restore Previous Config
      |
Restart / Restore Service
      |
Verify Previous Working State
```

Rollback failure is a distinct critical result. Preserve the journal and both
config artifacts, reject further mutations, and return a redacted
`ROLLBACK_FAILED` error requiring operator action.

### Crash Recovery Decision

The journal is durably written in a `prepared` phase before live replacement
and records old/candidate config hashes plus old/candidate runtime
`config_generation` values.
On startup, while holding the same lock:

- if SQLite is still at the old generation, restore the old config and old
  service state, even if the candidate had started successfully
- if SQLite is at the candidate generation, ensure the candidate config is live,
  restore its intended service state, verify, and finish cleanup
- if file/DB hashes or revisions match neither side, quarantine mutations and
  require operator action; never guess

SQLite commit atomicity is the commit decision. Journal cleanup is not.

## Concurrency

Only one config mutation transaction may run at a time, including across
multiple SubBox processes.

Concurrent requests that would modify the Sing-box config must serialize or fail cleanly.

The same lock is held until matching SQLite state and subscription content are
committed. Ordinary Refresh and public-port-only changes do not acquire the OS
config lock, but must still use SQLite transactions and optimistic node
revisions to prevent lost updates.

## File Safety

Use fixed trusted paths.

Expected paths:

- live config: `/etc/sing-box/config.json`
- SubBox state directory: `/etc/subbox/`
- backups: a fixed directory under `/etc/subbox/`

Candidate files should be written safely and replaced atomically where supported.

Use the same filesystem as the live config for atomic replacement. Reject
symlinks, preserve expected ownership/mode, fsync crash-sensitive writes, and
use a fixed backup location with bounded retention. The initial adoption of an
unmanaged live config is explicit and backed up; unsupported existing content
must never be silently discarded.

## Validation

A candidate configuration must pass `sing-box check` before replacing the live config.

A successful `sing-box check` alone is not enough: service restart/reload must also be verified.

Protocol adapters must declare and check installed-version capability before
generation. AnyTLS requires Sing-box 1.12.0 or later. The UI/API must report a
capability error rather than emit a config the installed binary cannot parse.
The installed binary's
`sing-box check -c <candidate>` remains authoritative.

TUIC `zero_rtt_handshake` defaults to disabled in SubBox because of replay risk.
Enabling it is outside the initial safe preset and requires a future explicit
advanced-setting contract.

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

Service mutations share the config transaction's cross-process lock to prevent
manual stop/restart from racing apply or rollback.

## Original Reference Repository

`caigouzi121380/singbox-deploy` may be consulted to understand deployment behavior, OS differences, supported protocol configuration, and URI patterns.

SubBox should implement its own structured Go modules rather than automating interactive `read -p` shell flows.
