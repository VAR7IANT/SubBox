# Security

## Principles

SubBox manages a privileged local service. The web layer must expose only narrowly scoped operations required for Sing-box management.

## Authentication

Authentication is a Phase 1 implementation prerequisite, not a release-time
afterthought. Until it exists, the backend must bind to loopback only and must
not be presented as remotely deployable.

Preferred baseline:

- server-side session
- HttpOnly cookie
- Secure cookie when served over HTTPS
- SameSite policy
- CSRF protection for state-changing requests
- login rate limiting

The initial administrator credential must not have a compiled-in/default
password. Password verifiers use a current password-hashing KDF. Sessions must
be random, server-side, revocable, idle-expiring, and rotated after login.
Logout invalidates the server-side session.

Initial password provisioning is local-only (for example, a fixed-purpose
interactive `subbox admin set-password` command). Do not expose an
unauthenticated remote bootstrap/setup endpoint.

State-changing requests require both a valid session and CSRF defense. Validate
`Origin`/`Host` and use a CSRF token; SameSite cookies alone are insufficient.
Reject non-JSON mutation requests unless an endpoint explicitly documents
another content type.

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

### Protocol Safety Defaults

Protocol adapters validate settings against the installed Sing-box version.
TUIC 0-RTT is disabled in the Phase 1 preset because it permits replay. TLS
certificate/key paths are trusted application-managed references, never raw
request-controlled filesystem paths.

## Filesystem

User input must not control:

- Sing-box config path
- backup directory
- database path
- service unit path

Prevent path traversal by using fixed trusted paths.

Baseline permissions:

- `/etc/subbox/`: `0700`, owned by the SubBox service account
- SQLite database, encryption key, transaction journal, and backups: `0600`
- live/candidate Sing-box config: least privilege required by the actual
  Sing-box service account (normally `0640` with a dedicated group), never
  world-readable

Atomic replacement must preserve intended owner/mode, fsync the file and parent
directory where required for crash durability, and reject symlink targets.
Backups require bounded retention and the same secret-safe permissions.

## Secrets

Do not write the following to ordinary logs:

- passwords
- Reality private keys
- subscription tokens
- session cookies
- generated credentials

Sensitive values may be returned only through authenticated user-facing endpoints when the product requires them.

Ordinary node list/detail responses are redacted views and must not include
passwords, private keys, raw subscription tokens, or encrypted credential
blobs. Error responses never include raw subprocess stderr. Any future reveal
endpoint must be separately specified, authenticated, re-confirmed, and
audited without recording the revealed value.

## Subscription Token

The public subscription token must be high entropy and non-sequential.

Consider:

- token revocation
- explicit regeneration
- rate limiting
- avoiding token exposure in application logs

Requirements for Phase 1:

- generate at least 32 random bytes using a cryptographic RNG and URL-safe
  encoding
- store a lookup hash and authenticated-encrypted token, never plaintext
- return invalid/disabled tokens as the same `404` response
- redact `/sub/<token>` in application and reverse-proxy access logs
- send `Referrer-Policy: no-referrer` and `Cache-Control: no-store`
- rate-limit failed token lookups without making the public feed unusable

Because the token is carried in the URL, operators must also configure any
fronting proxy/CDN not to log the full path. QR rendering should happen locally
in the authenticated UI; do not send the subscription URL to a third-party QR
service.

The authenticated `GET /api/subscription` response may decrypt the token only
to return the fixed URL required for Copy/QR. Mark that response `no-store`,
never include it in telemetry/error context, and keep it out of server logs.
The cached subscription body is also encrypted at rest because it contains
client credentials.

Token rotation is a separate operation from subscription content Refresh.

## Config Mutation

All live Sing-box config mutations must use the shared transaction manager defined in `SINGBOX_RULES.md`.

No handler or protocol module may bypass it.

The transaction lock must work across processes. It is held through the SQLite
commit and subscription publication. A database failure after live apply is a
transaction failure and triggers config/service rollback. Startup must recover
or clearly quarantine an incomplete durable transaction before accepting
mutations.

Candidate and backup filenames are generated by trusted code in fixed
directories. Subprocesses receive fixed executable paths and argument arrays,
run with deadlines, and execute with a minimal environment. Never invoke a
shell.

## Service Control

Only the Sing-box service is in scope.

Do not expose generic service-control endpoints accepting arbitrary service names.

Start/stop/restart operations acquire the same cross-process service/config
lock as config transactions so a manual service request cannot race an apply
or rollback.

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
- public subscription caching/referrer/access-log behavior
- credential encryption key provisioning and recovery
- loopback-only behavior before authentication is enabled
