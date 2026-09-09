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

Do not use ad-hoc shell cache files as the primary application database.

## Sing-box Manager Responsibilities

- inspect installed Sing-box version
- generate protocol-specific server config fragments
- generate credentials through safe mechanisms
- validate candidate config
- apply config via the Config Manager

## Config Manager Responsibilities

All live config mutations go through one transaction implementation:

`lock -> read -> backup -> build candidate -> temp write -> sing-box check -> atomic apply -> restart/reload -> verify -> commit`

On any failure after mutation begins, restore the previously working configuration when possible.

## Service Manager Responsibilities

Abstract Systemd and OpenRC behind a small interface for:

- status
- start
- stop
- restart

Do not expose arbitrary service names from user input.

## Subscription Service Responsibilities

- resolve stable subscription tokens
- read enabled node state
- generate client-facing URI output using `public_port`
- refresh content without changing credentials

## Phase 1 Deployment Model

Single-host deployment only.

Do not introduce a permanent remote monitoring agent or central control plane.
