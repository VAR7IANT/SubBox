# API Contract

This document defines the first-pass REST surface for SubBox. Sol should review and lock request/response schemas before backend implementation begins.

Base path: `/api`

## Nodes

### `GET /api/nodes`

Return all configured nodes.

### `POST /api/nodes`

Create a node.

Required concepts:

- `name`
- `protocol`
- `host`
- `listen_port`
- `public_port`
- protocol-specific settings

### `GET /api/nodes/:id`

Return one node.

### `PATCH /api/nodes/:id`

Update non-port node metadata/settings.

### `PATCH /api/nodes/:id/ports`

Update NAT-aware port fields.

Example request:

```json
{
  "listen_port": 443,
  "public_port": 24443
}
```

Behavior:

1. validate ports
2. build candidate config
3. run config transaction
4. persist application state only after successful apply
5. refresh subscription content

### `POST /api/nodes/:id/rotate`

Rotate protocol credentials.

This endpoint is intentionally separate from subscription refresh.

## Subscription

### `GET /api/subscription`

Return metadata for the current stable subscription, including its URL and last content refresh time.

### `POST /api/subscription/refresh`

Regenerate subscription content from current persisted node state.

Must NOT rotate credentials.

### Public subscription route

`GET /sub/:token`

Returns subscription content for a valid stable token.

The token must not change just because node ports or credentials changed.

## Config

### `GET /api/config/status`

Return lightweight Sing-box configuration validity/status information.

This is an on-demand status read, not background monitoring.

## Service

### `GET /api/service/status`

Return Sing-box service status.

### `POST /api/service/start`

Start Sing-box.

### `POST /api/service/stop`

Stop Sing-box.

### `POST /api/service/restart`

Restart Sing-box.

## Error Shape

Prefer one stable structured shape, for example:

```json
{
  "error": {
    "code": "INVALID_PORT",
    "message": "public_port must be between 1 and 65535"
  }
}
```

Do not leak shell output, private keys, passwords, or internal filesystem details to unauthenticated clients.

## Contract Rules

- Do not use one ambiguous `port` field.
- `listen_port` is server-local.
- `public_port` is client-facing.
- Refresh and Rotate must remain separate API operations.
- No generic `/exec`, shell, terminal, or arbitrary-command endpoint.
