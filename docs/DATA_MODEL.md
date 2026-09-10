# Data Model

This is the reviewed Phase 1 logical model. Physical migrations must preserve
these boundaries; exact SQL and key provisioning still require a focused Sol
task before backend implementation.

## Node

Logical fields:

```text
id              string / UUID
name            string
protocol        enum
host            string
listen_port     integer
public_port     integer
enabled         boolean
credentials     protocol-specific structured data
protocol_config versioned, strictly validated non-secret settings
revision        integer
created_at      timestamp
updated_at      timestamp
```

### Protocol enum

Initial values:

- `shadowsocks`
- `hysteria2`
- `tuic`
- `vless_reality`
- `anytls`

### Port semantics

`listen_port`:

The local port used by Sing-box in `/etc/sing-box/config.json`.

`public_port`:

The port written into client-facing URIs and subscriptions after NAT / port forwarding.

Never replace these two fields with one generic `port` column.

`host` is the public client-facing host used in URIs. It is not a local bind
address.

`public_port` is declarative; SubBox does not claim that an external NAT rule
exists. Enabled listener conflicts are validated using the protocol adapter's
derived transport and trusted bind address. Potential public mapping conflicts
may be warned about, but the database must not assume control of the external
NAT device.

`revision` starts at 1 and increments on committed node changes, including
credential rotation. It supports optimistic concurrency in mutating APIs.

## Subscription

Logical fields:

```text
id                  string / UUID
token_hash          hash of a high-entropy opaque token
token_ciphertext    encrypted token for authenticated URL display
enabled             boolean
content_ciphertext  encrypted last committed subscription snapshot
content_revision    monotonically increasing integer
content_updated_at  timestamp
created_at          timestamp
updated_at          timestamp
```

Rules:

- token remains stable across node edits
- token remains stable across port changes
- token remains stable across Refresh
- token remains stable across Rotate
- explicit token regeneration/revocation can be designed separately

The raw token is generated from at least 32 random bytes with a URL-safe
encoding. Store a keyed/cryptographic lookup hash plus an
authenticated-encrypted copy. The hash resolves public requests without
decrypting every row; the encrypted copy lets the authenticated UI reconstruct
the same fixed URL after restart. Never store or log plaintext. Token lookup
must use constant-time comparison where practical.

`content_ciphertext` and `content_updated_at` are updated in the same SQLite
transaction as any node state change that affects client output. After token
authorization, `GET /sub/:token` decrypts and serves this snapshot.
`content_revision` increments only when a new snapshot is published; Refresh
may increment it without changing any node revision.

## Settings

Logical singleton fields/concepts:

```text
id / singleton key
public_base_url
ui_language
config_adopted_at / nullable ownership marker
created_at
updated_at
```

`public_base_url` is canonical trusted configuration, not request-derived. It
must be an absolute HTTPS URL in production (loopback HTTP is allowed for
development) and must not contain userinfo, query, or fragment.

Do not store monitoring-agent configuration because SubBox has no monitoring agent.

## Runtime State

```text
id                singleton key
config_generation monotonically increasing integer
live_config_hash  hash of the last committed generated config
updated_at        timestamp
```

Every successful live-config commit increments `config_generation` and stores
the matching hash in the same SQLite transaction as node changes and the
subscription snapshot. The durable journal records old/candidate generations;
this singleton, rather than a guess across individual node revisions, is the
crash-recovery commit decision.

## Credentials

Protocol-specific credential structures should be typed in Go rather than treated as arbitrary unvalidated JSON wherever practical.

Phase 1 stores non-secret protocol settings as canonical versioned JSON and
credentials as an authenticated-encrypted versioned blob. Decoding must be
strict by protocol and schema version. Arbitrary JSON passed through from an
API request is not acceptable. API views are typed and never expose storage
blobs.

Examples:

### Shadowsocks

- method
- password

### Hysteria2

- password
- optional obfuscation password (stored separately from the authentication
  password so Rotate does not change it)
- TLS-related settings required by the server config

### TUIC

- uuid
- password

### VLESS Reality

- uuid
- server_name / SNI
- private key
- public key
- short ID

### AnyTLS

- user display name
- password
- TLS server name and certificate/key reference

## TLS Material

Phase 1 supports only locally imported existing certificate/private-key
material. A future fixed-purpose, local-only CLI (for example,
`subbox tls import`) will copy the certificate and private key into
SubBox-managed trusted storage. The actual managed location is selected by the
trusted implementation; it is never supplied as an HTTP request or arbitrary
API path.

Protocol configuration references TLS material only by a canonical logical
`tls_material_id`. It must not store `/etc/foo/key.pem` or any other
request-controlled certificate/private-key path. Phase 1 does not support ACME,
automatic certificate generation, or a remote certificate upload API.

Ordinary credential Rotate changes protocol credentials only and never rotates
the certificate/private-key material referenced by `tls_material_id`.

TLS private keys and any certificate/private-key backups remain secret
material and follow the same secret-handling, file-permission, and
service-account access rules as other SubBox-managed secrets.

Credential storage requires a security review before release. Secrets must never be included in ordinary application logs.

At rest, credential confidentiality must not depend on SQLite file permissions
alone. Use authenticated encryption with a key stored separately from the
database and readable only by the SubBox service account. Key provisioning and
backup/recovery behavior must be finalized before the credential migration is
implemented.

## Authentication

```text
admin_user
  id                 singleton UUID
  password_verifier  password-KDF output
  password_changed_at timestamp
  created_at          timestamp
  updated_at          timestamp

session
  id_hash             hash of random session ID
  csrf_secret_hash    hash of random CSRF secret
  created_at          timestamp
  last_seen_at        timestamp
  expires_at          timestamp
  revoked_at          nullable timestamp
```

Only hashes/verifiers are stored. Session and CSRF plaintext values exist only
in the browser cookie/header and request handling memory. Expired/revoked
sessions are removed with bounded cleanup. Phase 1 has one administrator and
does not add roles or tenants.

## Persistence Rule

Application state should be committed only after a Sing-box configuration transaction succeeds where the state change affects the live proxy configuration.

The node change and regenerated subscription snapshot are one SQLite
transaction. If that commit fails after a candidate live config has been
activated, the Config Manager rolls the live config and service state back to
the pre-transaction snapshot.

## Config Transaction Journal

A minimal durable journal under `/etc/subbox/` records:

- transaction ID and phase
- trusted backup/candidate identifiers (never request-controlled paths)
- whether the service was active before mutation
- hash and runtime `config_generation` of the old and candidate configs

It must not contain credentials or raw subscription tokens. Startup recovery
must resolve an incomplete journal before accepting another config mutation.
