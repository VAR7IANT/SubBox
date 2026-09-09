# Architecture

## Overview

SubBox should keep UI, application logic, persistence, Sing-box configuration, and OS service control separated.

```text
Browser
  |
React + TypeScript
  |
REST API
  |
Go HTTP Handlers
  |
Application Services
  |---------------------------|
  |                           |
Node / Subscription Domain   Sing-box Manager
  |                           |
SQLite                      Config Manager
                              |
                         Service Manager
                              |
                           sing-box
```

## Proposed Repository Layout

```text
frontend/
  src/
    api/
    components/
    features/
      subscription/
      nodes/
      deploy/
      relay/
      settings/
    pages/
    types/

backend/
  cmd/subbox/
    main.go
  internal/
    api/
    service/
    domain/
    storage/
    subscription/
    singbox/
    protocol/
    system/

  migrations/

docs/
```

The exact file breakdown may evolve after the first Sol architecture review, but layer responsibilities must remain clear.

## Frontend Responsibilities

- Render the UI reference faithfully.
- Own presentation state, forms, confirmation dialogs, loading/error states.
- Call typed API functions from `frontend/src/api/`.
- Never attempt direct OS or Sing-box operations.

## HTTP Handler Responsibilities

Handlers should:

- parse requests
- validate request shape
- call application services
- translate errors into HTTP responses

Handlers must not directly edit `/etc/sing-box/config.json`, invoke `systemctl`, or contain protocol generation logic.

## Application Service Responsibilities

Services coordinate use cases such as:

- create/update node
- change ports
- refresh subscription
- rotate credentials
- apply config transaction
- restart Sing-box

## Storage Responsibilities

SQLite stores SubBox-owned application state such as node metadata, public ports, stable subscription token, and settings.

SQLite is the source of truth for the last committed desired state. The live
Sing-box file and the published subscription body are derived artifacts. A
change is not committed merely because a candidate config started
successfully; the config transaction must also commit the matching SQLite
state and subscription snapshot.

Do not use ad-hoc shell cache files as the primary application database.

## Live Config Ownership

Phase 1 uses exclusive, whole-file ownership of
`/etc/sing-box/config.json` after an explicit adoption/deploy step. SubBox must
not silently overwrite an existing unmanaged file.

- Detection/status reads are allowed before adoption.
- The first adoption/deploy creates a durable backup of any existing file.
- If an existing config cannot be represented by the supported Phase 1 model,
  adoption must fail with a clear error instead of dropping unknown content.
- After adoption, every write is generated from one committed SQLite snapshot
  and goes through the shared Config Manager.

Managed-section merging can be designed later; Luna must not invent it during
Phase 1.

## Sing-box Manager Responsibilities

- inspect installed Sing-box version
- generate protocol-specific server config fragments
- generate credentials through safe mechanisms
- validate candidate config
- apply config via the Config Manager

## Config Manager Responsibilities

All live config mutations go through one transaction implementation:

`validate -> process lock -> recover incomplete transaction -> snapshot old state -> build candidate -> durable temp write -> sing-box check -> durable backup -> atomic apply -> restore prior service state -> verify -> commit DB + subscription snapshot -> cleanup`

The lock covers config replacement, service control, SQLite commit, and
subscription publication. It must coordinate across processes, not only Go
goroutines.

On any failure after the live file changes, including a SQLite commit failure,
restore the previous config and previous service state and verify recovery. A
small durable transaction marker is required so startup can detect and recover
an interrupted mutation.

A change to `public_port` alone is not a live config mutation. It atomically
updates SQLite and the subscription snapshot without invoking Sing-box or the
service manager.

## Service Manager Responsibilities

Abstract Systemd and OpenRC behind a small interface for:

- status
- start
- stop
- restart

Do not expose arbitrary service names from user input.

Mutating service operations acquire the same cross-process lock used by the
Config Manager. Status remains a read-only operation.

## Subscription Service Responsibilities

- resolve stable subscription tokens
- read enabled node state
- generate client-facing URI output using `public_port`
- refresh content without changing credentials
- publish a complete subscription snapshot atomically with the committed node
  state

`GET /sub/:token` serves only the last committed snapshot. It must never
assemble content from partially updated state. The stable URL is derived from
the configured public base URL plus the immutable subscription token; only an
explicit base-URL or token-management operation may change it.

## Phase 1 Deployment Model

Single-host deployment only.

Do not introduce a permanent remote monitoring agent or central control plane.
