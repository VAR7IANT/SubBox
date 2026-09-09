# API Contract

This document defines the reviewed first-pass REST surface and behavioral
contract. Endpoint schemas are locked only where stated explicitly; deferred
flows at the end of this document require another Sol review before coding.

Base path: `/api`

All `/api` routes require an authenticated panel session. State-changing
routes also require CSRF protection. `GET /sub/:token` is the sole public route
in this contract; possession of its high-entropy token is the authorization.

JSON responses must use `Content-Type: application/json`. Subscription output
uses the format-specific content type selected by the implementation and
`Cache-Control: no-store` in Phase 1.

## Authentication

### `POST /api/auth/login`

Accept an administrator password over HTTPS, apply login rate limiting, rotate
the server-side session ID, and set the session cookie. The response includes a
CSRF token for subsequent state-changing requests. Authentication failures use
one generic `401 INVALID_CREDENTIALS` response.

### `POST /api/auth/logout`

Require session + CSRF, invalidate the server-side session, and expire the
cookie.

### `GET /api/auth/session`

Return authenticated session state and a CSRF token. It never returns a
password verifier or session identifier.

The exact cookie name is application-owned. It is `HttpOnly`, `SameSite=Strict`
by default, scoped narrowly, and `Secure` outside loopback development. Mutating
requests send the CSRF token in a dedicated header; it must not appear in URLs.

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

`host` is the client-facing public DNS name or IP address. It is never used as
the Sing-box listen address. Phase 1 inbounds listen on a trusted,
application-defined address (normally all interfaces); a configurable bind
address requires a future contract change.

Creation is a state-changing operation. For an enabled node it uses the shared
config transaction, then commits the node and subscription snapshot. A disabled
node is stored without adding an inbound and is omitted from the subscription.
Generated credentials are never returned in the ordinary response.

### `GET /api/nodes/:id`

Return one node.

### `PATCH /api/nodes/:id`

Update non-port node metadata/settings.

The request includes the expected node `revision`. Name and public `host`
changes update SQLite plus the subscription snapshot only. Changes to enabled
state or protocol/server settings run the shared config transaction. Credential
fields are not accepted here; use Rotate for generated authentication
credentials.

### `PATCH /api/nodes/:id/ports`

Update NAT-aware port fields.

Example request:

```json
{
  "revision": 7,
  "listen_port": 443,
  "public_port": 24443
}
```

Behavior:

1. validate ports
2. if `listen_port` changed, run the shared config transaction and commit the
   node update plus subscription snapshot only after service verification
3. if only `public_port` changed, do not touch the Sing-box config or service;
   atomically update the node and subscription snapshot in SQLite
4. leave the stable subscription token unchanged in both cases

Requests must include the node's last observed `revision`. A stale revision
returns `409 REVISION_CONFLICT` rather than overwriting a concurrent update.

### `DELETE /api/nodes/:id`

Delete one node using its expected `revision`. If enabled, remove its inbound
through the shared config transaction. Commit deletion and the new subscription
snapshot only after verification. If disabled, use one SQLite transaction and
do not touch Sing-box. Deletion does not rotate the subscription token.

### `POST /api/nodes/:id/rotate`

Rotate protocol credentials.

This endpoint is intentionally separate from subscription refresh.

Phase 1 rotation is node-scoped. It rotates only the addressed node, applies
one config transaction, and atomically publishes the matching subscription
snapshot. It never changes the subscription token. The request must include
the expected node `revision` and explicit confirmation, for example:

```json
{
  "revision": 7,
  "confirm": true
}
```

Ordinary node and rotation responses must not return server-only secrets such
as passwords or Reality private keys. Authenticated, deliberate credential
reveal/export requires a separate future contract.

## Subscription

### `GET /api/subscription`

Return metadata for the current stable subscription, including its URL and last content refresh time.

This authenticated response may contain the full token-bearing URL because it
exists specifically for Copy/QR. It must use `Cache-Control: no-store` and must
not be logged. The URL is built only from configured `public_base_url` and the
stored encrypted token, never from an untrusted request `Host` or forwarded
header.

`public_base_url` is validated trusted configuration: an absolute HTTPS URL in
production (HTTP is allowed only for loopback development), with no userinfo,
query, or fragment. The server never infers it from request headers.

### `POST /api/subscription/refresh`

Regenerate subscription content from current persisted node state.

Must NOT rotate credentials.

Behavior:

1. read one committed SQLite snapshot
2. regenerate the complete subscription body
3. atomically replace the cached body and update `content_updated_at`
4. keep node credentials, node revisions, token, and URL unchanged

Refresh never invokes the Config Manager or Service Manager.

### Public subscription route

`GET /sub/:token`

Returns subscription content for a valid stable token.

The token must not change just because node ports or credentials changed.
The response is the last atomically published snapshot; failed node/config
transactions must never become visible here.

Phase 1 does not expose `POST /api/subscription/rotate`. A global Rotate button
must not imply that all nodes are rotated; the UI must choose a node and call
the node-scoped endpoint.

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

All service mutations acquire the same cross-process lock as config
transactions. They return `409 CONFIG_MUTATION_BUSY` (or wait within a bounded
server timeout) rather than racing apply/rollback.

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

Minimum status/code mapping:

- `400` malformed request
- `401 AUTHENTICATION_REQUIRED`
- `403 CSRF_FAILED` or authenticated-but-forbidden operation
- `404 NOT_FOUND` (also used for an invalid subscription token)
- `409 REVISION_CONFLICT` or `CONFIG_MUTATION_BUSY`
- `422 VALIDATION_FAILED`
- `500 CONFIG_APPLY_FAILED` or `ROLLBACK_FAILED`

Public errors must use a safe message. Detailed command output belongs only in
redacted privileged logs.

## Contract Rules

- Do not use one ambiguous `port` field.
- `listen_port` is server-local.
- `public_port` is client-facing.
- Refresh and Rotate must remain separate API operations.
- No generic `/exec`, shell, terminal, or arbitrary-command endpoint.

## Deferred Contracts

The deploy/import and public-base-URL settings flows remain in product scope,
but their request/response schemas are not yet locked. No backend handler for
those flows may be inferred from this document. Sol must add a focused contract
review before assigning those implementations. The same gate applies to TLS
certificate provisioning/import required by Hysteria2, TUIC, and AnyTLS.
