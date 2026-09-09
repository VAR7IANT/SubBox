# Security

## Principles

SubBox manages a privileged local service. The web layer must expose only narrowly scoped operations required for Sing-box management.

## Authentication

Before production release, the panel must have authenticated access.

Preferred baseline:

- server-side session
- HttpOnly cookie
- Secure cookie when served over HTTPS
- SameSite policy
- CSRF protection for state-changing requests
- login rate limiting

Do not build a large multi-tenant identity system in Phase 1.

## Command Execution

Never concatenate user-controlled input into a shell string.

Bad:

```go
exec.Command("sh", "-c", "sing-box check -c " + userInput)
```

Preferred:

```go
exec.CommandContext(ctx, "sing-box", "check", "-c", trustedConfigPath)
```

Executable names and filesystem paths used for privileged operations must come from trusted application configuration, not arbitrary request input.

## Input Validation

Validate at API boundaries and again where security-sensitive operations require it.

### Ports

- integer only
- range `1..65535`

### Host / Domain / SNI

- enforce reasonable maximum length
- reject control characters
- reject shell metacharacter assumptions by never using shell interpolation
- normalize carefully where needed

### UUID

Validate expected UUID format for protocols that require it.

### Names

Node names are presentation metadata and must not become filesystem paths or shell fragments.

## Filesystem

User input must not control:

- Sing-box config path
- backup directory
- database path
- service unit path

Prevent path traversal by using fixed trusted paths.

## Secrets

Do not write the following to ordinary logs:

- passwords
- Reality private keys
- subscription tokens
- session cookies
- generated credentials

Sensitive values may be returned only through authenticated user-facing endpoints when the product requires them.

## Subscription Token

The public subscription token must be high entropy and non-sequential.

Consider:

- token revocation
- explicit regeneration
- rate limiting
- avoiding token exposure in application logs

Token rotation is a separate operation from subscription content Refresh.

## Config Mutation

All live Sing-box config mutations must use the shared transaction manager defined in `SINGBOX_RULES.md`.

No handler or protocol module may bypass it.

## Service Control

Only the Sing-box service is in scope.

Do not expose generic service-control endpoints accepting arbitrary service names.

## No Monitoring Agent

SubBox must not install a persistent telemetry/monitoring agent or periodically report host metrics to a central server.

## Security Review Gate

Before the first production release, Sol review must cover:

- authentication/session behavior
- CSRF
- rate limiting
- command injection
- path traversal
- secret logging
- file permissions
- SQLite permissions
- config backup permissions
- rollback behavior
