# SubBox Development Rules

## Product

SubBox is a lightweight Sing-box deployment, node management, NAT port mapping, and subscription management application.

It is NOT a server monitoring platform.

## Source of Truth

Read these files before architectural or behavioral changes:

1. `docs/PROJECT_SPEC.md`
2. `docs/ARCHITECTURE.md`
3. `docs/API_CONTRACT.md`
4. `docs/DATA_MODEL.md`
5. `docs/UI_SPEC.md`
6. `docs/SECURITY.md`
7. `docs/SINGBOX_RULES.md`
8. `docs/TASKS.md`

Do not silently change documented contracts.

## Technology

- Frontend: React + TypeScript + Vite + Tailwind CSS + shadcn/ui
- Backend: Go
- Storage: SQLite
- Sing-box config: `/etc/sing-box/config.json`
- SubBox state: `/etc/subbox/`

## Current Scope

- Sing-box deployment
- Node management
- Stable subscription URL
- Copy / QR
- Refresh subscription content
- Rotate credentials
- `listen_port`
- `public_port`
- NAT / port mapping
- Config validation
- Service start / stop / restart

Protocols:

- Shadowsocks
- Hysteria2
- TUIC
- VLESS Reality
- AnyTLS

Do not expand scope unless explicitly instructed.

## Never Add Without Explicit Product Decision

- Server monitoring
- CPU / RAM / disk history
- Ping / uptime monitoring
- Traffic-history dashboard
- Monitoring Agent
- Telemetry collector
- Remote Shell
- Web Terminal
- File Manager
- Arbitrary command API
- Remote job execution

## Ports

Never model a node with one ambiguous `port` field.

- `listen_port`: local port Sing-box listens on.
- `public_port`: client-facing port after NAT / port forwarding.

Example:

`public :24443 -> NAT -> local :443`

means:

- `listen_port = 443`
- `public_port = 24443`

Sing-box server configuration uses `listen_port`.
Client URIs and subscriptions use `public_port`.

## Subscription

Subscription URLs must remain stable when nodes change.

### Refresh

Regenerate subscription content from the current state.
Do not rotate credentials.

### Rotate

Generate new protocol credentials, update Sing-box safely, validate, apply, restart/reload, verify, then refresh subscription content.

Refresh and Rotate must never be merged into one operation.

## Sing-box Config Safety

Never overwrite the live config without validation.

All changes must use:

`validate -> backup -> build candidate -> write temp -> sing-box check -> apply -> restart/reload -> verify`

On failure:

`rollback -> restore old config -> restore service`

## Security

Never concatenate user-controlled input into shell commands.
Never expose a generic command execution API.
Validate ports, domains, IPs, SNI, UUIDs, and names.
Do not log passwords, private keys, subscription tokens, or credentials unless required for a secure user-facing action.

## UI

Reference image: `docs/ui/subbox-ui-orange.png`

Primary style:

- pale orange / apricot
- warm white / cream
- minimal and modern
- rounded cards
- subtle shadows
- green primarily for running/success

Do not redesign the application into a monitoring dashboard.

## Development

Prefer minimal scoped changes.
Do not refactor unrelated code.
Do not change architecture, API contracts, database schema, or technology stack unless the task explicitly requires it.

Every completed task should report:

- changed files
- implemented functionality
- build result
- lint result
- typecheck result
- test result
- remaining issues
