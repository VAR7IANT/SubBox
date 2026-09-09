# Project Specification

## Product Definition

SubBox is an independent lightweight web application for deploying and managing Sing-box nodes, exposing client subscriptions, and handling NAT / port-forwarding scenarios.

The public reference project is `caigouzi121380/singbox-deploy`. It is used to understand proven deployment flows and protocol configuration patterns. SubBox should not become a browser wrapper that drives the original interactive shell script.

## Primary User Flow

1. Deploy or import a Sing-box node.
2. Configure its local listening port and client-facing public port.
3. Expose the node through one stable subscription URL.
4. Copy or scan the subscription URL once on the client.
5. Change ports, edit nodes, refresh subscription content, or rotate credentials from the Web UI.
6. The subscription URL itself remains unchanged.

## Phase 1 Scope

- Visual node list and node editing
- Deployment flow for supported protocols
- Stable subscription URL
- Copy subscription URL
- QR code
- Refresh subscription content
- Rotate credentials as a separate destructive action
- Change `listen_port`
- Change `public_port`
- NAT / port mapping support
- Sing-box config validation
- Start / stop / restart service actions

Supported protocols:

- Shadowsocks
- Hysteria2
- TUIC
- VLESS Reality
- AnyTLS

## Explicit Non-Goals

SubBox is not a Nezha-style monitoring or remote-operations platform.

Phase 1 must not contain:

- monitoring agents
- periodic machine telemetry
- CPU / RAM / disk history
- ping / uptime charts
- server traffic history dashboard
- remote shell or web terminal
- file manager
- arbitrary command execution
- remote task scheduler
- multi-server monitoring fleet

Instant Sing-box-specific status reads are acceptable when requested by the UI, but they must not become background monitoring.

## Subscription Model

A subscription is represented by a stable opaque token, for example:

`https://example.com/sub/<token>`

Changing node configuration, ports, or credentials must not automatically change the subscription token.

In Phase 1, “stable URL” means that both the opaque token and `/sub/<token>`
path remain unchanged across normal operations. Changing the configured public
base URL is a separate explicit settings action and naturally changes the
displayed absolute URL; it does not rotate the token. The public endpoint
always serves the last fully committed subscription snapshot.

### Refresh

Refresh regenerates subscription output from the current persisted node state. It must not rotate authentication credentials.

### Rotate

Rotate changes protocol credentials, safely applies a candidate Sing-box configuration, verifies the service, then updates subscription output.

Phase 1 Rotate is explicit and node-scoped. It does not regenerate the stable
subscription token.

## NAT Model

SubBox must model local listening and public client-facing ports separately.

Example:

- Sing-box listens locally on `443`.
- VPS/NAT panel maps public `24443` to local `443`.
- `listen_port = 443`
- `public_port = 24443`
- generated client URI uses `24443`.

In Phase 1, `public_port` is declarative metadata supplied by the operator.
SubBox does not call a VPS provider API and does not create firewall, router,
UPnP, or NAT rules. The operator owns the actual mapping. Changing
`listen_port` therefore never assumes the external mapping changed, and
changing only `public_port` never changes the local listener.

## UI Direction

Reference: `docs/ui/subbox-ui-orange.png`

The UI should feel warm, lightweight, and product-focused rather than infrastructure-monitoring-focused.

Primary visual language:

- pale orange / apricot
- cream / warm white
- restrained warm gray
- green only for healthy/running state
- generous whitespace
- rounded cards
- subtle shadows

## Implementation Principle

Prefer clean-room independent implementation of SubBox behavior. Do not copy large sections of external source code merely because they already exist. When referencing third-party implementation details, respect the source project's license and attribution requirements.
