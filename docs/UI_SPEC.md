# UI Specification

## Reference

Primary visual reference:

`docs/ui/subbox-ui-orange.png`

The reference image defines the initial visual direction, not an immutable pixel-perfect contract.

## Visual Direction

SubBox should feel like a lightweight product UI, not a server monitoring dashboard.

Palette direction:

- pale orange / apricot accent
- warm white / cream page background
- very light warm gray borders
- dark slate primary text
- softer warm gray secondary text
- green reserved mainly for Running / Success

Avoid:

- heavy dark panels
- neon colors
- strong blue-black monitoring-dashboard styling
- Grafana-like charts

## Layout

Primary desktop layout:

- fixed left sidebar
- top header/status area
- main content area
- card-based information hierarchy

Sidebar items:

- 总览
- 节点
- 部署
- 中转
- 设置

## Dashboard Priorities

The visual priority should be:

1. Subscription
2. Nodes
3. Lightweight Sing-box config/service state

Do not prioritize machine telemetry.

## Subscription Card

Must include:

- stable subscription URL
- Copy
- QR
- Refresh
- Rotate Credentials
- last content update time
- node count

Refresh and Rotate must be visually distinct. Rotate should look more deliberate/destructive than Refresh and should require confirmation.

## Node Card

Each node card should show at minimum:

- protocol name
- running/enabled state
- host
- `listen_port`
- `public_port`
- clear NAT/mapping relationship when ports differ

Primary actions:

- Copy Link
- Edit
- Change Port

Optional node-level Rotate can be exposed when the backend contract is ready.

## Port Editing UI

Change-port UI must expose both fields independently:

- 监听端口 / `listen_port`
- 对外端口 / `public_port`

The UI should help users understand a mapping such as:

`public 24443 -> local 443`

Changing only `public_port` should not imply that Sing-box's listening port changes.

## Core Components

Initial component set:

- `AppShell`
- `Sidebar`
- `Header`
- `SubscriptionCard`
- `NodeCard`
- `ConfigStatusCard`
- `ProtocolBadge`
- `QRModal`
- `ChangePortModal`
- `EditNodeModal`
- `RotateConfirmDialog`
- `Toast`

## Interaction States

Important operations should support:

- idle
- loading
- success
- validation error
- server error
- disabled
- confirmation for destructive operations

## Responsive Direction

Phase 1 is desktop-first, but layouts should not make mobile adaptation impossible. Avoid hard-coding the UI to a single screenshot size.
