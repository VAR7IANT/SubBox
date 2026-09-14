# Tasks

This file is the execution queue. Sol owns task decomposition and review. Luna implements one task at a time.

## Task 001 — Frontend Foundation

**Owner:** Luna

### Goal

Create the initial frontend project using React + TypeScript + Vite + Tailwind CSS. Prepare a clean project structure that can support shadcn/ui components later. Render only a minimal application placeholder; Task 002 owns the real app shell.

### Allowed Scope

- `frontend/**`
- root package/tooling files only when strictly required for the frontend bootstrap

### Do Not Change

- API contract
- data model
- backend architecture
- product scope

### Contract / Behavior References

- `docs/ARCHITECTURE.md` — frontend responsibilities and repository layout
- `docs/UI_SPEC.md` — visual direction and no-monitoring boundary
- `AGENTS.md` — required technology and development rules

### Acceptance Criteria

- frontend builds successfully
- TypeScript strict mode enabled
- a minimal application placeholder renders (the full shell is intentionally deferred)
- no monitoring/dashboard-chart dependencies added
- one package manager and its lockfile are committed; no mixed lockfiles
- lint and typecheck scripts are present and pass

### Verification

- run the chosen package manager's clean install command
- run `build`
- run `typecheck`
- run `lint`

---

## Task 002 — App Shell and Sidebar

**Owner:** Luna

### Goal

Implement the static application shell based on `docs/ui/subbox-ui-orange.png`.

### Precondition

Confirm the reference PNG renders in the development/browser toolchain before
starting visual matching. If it does not, stop and report the asset problem;
do not invent a replacement visual direction.

Include:

- sidebar
- header
- page content container
- active navigation state
- warm orange/cream visual tokens

Sidebar items:

- 总览
- 节点
- 部署
- 中转
- 设置

### Allowed Scope

- `frontend/src/**`
- frontend styling/config files strictly required for tokens

### Do Not Change

No backend/API work.

Do not add mock CPU/RAM/disk/ping/traffic widgets, routing libraries that are
not needed for the static shell, or unrelated dependencies.

### Contract / Behavior References

- `docs/UI_SPEC.md` — layout, palette, navigation, responsive direction
- `docs/ui/subbox-ui-orange.png` — visual reference

### Acceptance Criteria

- layout visually follows the reference
- no CPU/RAM/disk/ping/traffic monitoring UI
- responsive enough not to depend on one exact screenshot width
- keyboard focus remains visible and navigation controls have accessible names

### Verification

- run `build`
- run `typecheck`
- run `lint`
- manually inspect desktop width and one narrow viewport

---

## Task 003 — Subscription Card Mock UI

**Owner:** Luna

### Goal

Implement a static/mock `SubscriptionCard` with:

- stable subscription URL
- Copy button
- QR button
- Refresh button
- Rotate Credentials button
- last updated metadata
- node count

Use mock data only.

The Rotate entry is only a node-scoped flow launcher. Its mock dialog must
require selecting exactly one node and confirming; it must not imply
rotate-all behavior.

### Allowed Scope

- `frontend/src/features/subscription/**`
- shared UI primitives/types strictly needed by this feature
- frontend dependency metadata only if a small QR library is required

### Do Not Change

- no API calls or backend work
- no credential generation
- no global/all-node Rotate behavior
- no third-party QR web service

### Contract / Behavior References

- `docs/API_CONTRACT.md` — Refresh and node-scoped Rotate boundaries
- `docs/UI_SPEC.md` — Subscription Card
- `docs/SECURITY.md` — local QR generation and secret handling

### Behavioral UI Rule

Refresh and Rotate must look and behave as distinct actions. Rotate requires confirmation in the UI flow.

### Acceptance Criteria

- copy interaction works in browser
- QR dialog can open using mock subscription URL
- Refresh has loading/success mock state
- Rotate requires one mock node selection and a confirmation dialog
- Refresh never changes the displayed URL or mock credentials
- Rotate never changes the displayed subscription URL
- QR is generated locally in the browser

### Verification

- run `build`
- run `typecheck`
- run `lint`
- exercise Copy, QR, Refresh, cancel Rotate, and confirm Rotate mock paths

---

## Task 004 — Node Cards and NAT Port UI

**Owner:** Luna

### Goal

Implement reusable static/mock node cards for:

- VLESS Reality
- Hysteria2
- AnyTLS

The shared protocol type/badge mapping must cover all five Phase 1 enum values,
including Shadowsocks and TUIC, even though only three representative cards are
required on the mock page.

Each card must display:

- protocol
- status
- host
- `listen_port`
- `public_port`
- mapping relationship when ports differ

Actions:

- Copy Link
- Edit
- Change Port

### Allowed Scope

- `frontend/src/features/nodes/**`
- shared UI primitives/types and mock fixtures strictly needed by this feature

### Do Not Change

- no backend/API work
- no real service/config mutations
- do not collapse the two ports into one `port` value

### Contract / Behavior References

- `docs/DATA_MODEL.md` — node fields and port semantics
- `docs/SINGBOX_RULES.md` — NAT and URI rules
- `docs/UI_SPEC.md` — Node Card

### Acceptance Criteria

The UI clearly communicates that `public_port` may differ from `listen_port`.

Node/card types use the exact `listen_port` and `public_port` property names.
The displayed client link uses `host:public_port`, never `listen_port` when the
two differ. Server-only credentials are absent from mock list/detail models.
All five Phase 1 protocol enum values render without falling back to an unknown
label.

### Verification

- run `build`
- run `typecheck`
- run `lint`
- inspect equal-port and NAT-mapped fixtures

---

## Task 005 — Port Editing Interaction

**Owner:** Luna

### Goal

Implement the frontend Change Port dialog using mock state.

Fields:

- `listen_port`
- `public_port`

Validation:

- integer
- 1..65535

### Allowed Scope

- `frontend/src/features/nodes/**`
- shared form/dialog primitives strictly needed by this feature

### Do Not Change

- no backend/API integration
- no API contract, schema, or dependency changes unrelated to the dialog
- no Rotate or subscription-refresh implementation

### Contract / Behavior References

- `docs/API_CONTRACT.md` — `PATCH /api/nodes/:id/ports` behavior
- `docs/SINGBOX_RULES.md` — public-only versus listen-port changes
- `docs/UI_SPEC.md` — Port Editing UI

### Important

Changing only `public_port` must not imply that `listen_port` changed.

### Acceptance Criteria

- validation errors are visible
- save/cancel states work
- successful mock update refreshes the node card
- no real backend call yet unless Sol has explicitly updated this task
- changing only `public_port` preserves `listen_port` byte-for-byte in mock state
- the confirmation summary distinguishes “subscription-only mapping change”
  from “Sing-box listener change”

### Verification

- run `build`
- run `typecheck`
- run `lint`
- test non-integer, 0, 65536, public-only, listen-only, and both-port edits

---

# Review Gate

After Tasks 001–005, stop feature expansion.

Sol should review:

- visual direction
- component boundaries
- port semantics
- absence of monitoring behavior
- readiness for API integration

Backend implementation tasks should be added only after this review.

Do not begin backend migrations, config management, authentication, or real API
integration from an inferred design. Sol must add narrowly scoped tasks after
the gate, beginning with persistence/authentication foundations and the shared
config transaction before any handler can mutate live Sing-box state.

Architecture gate status after the first Sol review: Tasks 001–005 are approved
to run in order. Backend handlers remain blocked until Sol adds focused tasks
for SQL migrations/key provisioning, authentication, protocol schemas, and the
shared crash-recoverable config transaction. Deploy/import and settings API
schemas are explicitly deferred in `API_CONTRACT.md`.

---

# Backend Task Ledger Reconciliation — 2026-09-15

Repository review found that `main` had already completed the backend foundation work below while this execution queue still stopped at Task 005. The entries below reconcile the queue with committed repository history. Historical completed tasks are recorded at summary level; their implementation is already present on `main` and must not be reimplemented.

## Task 006 — Backend Persistence Foundation — COMPLETE

**Committed baseline:** `2fbf474012482fb2938bc93872a6a6672efc3cc1`

Established the SQLite persistence foundation, initial migration/state layout, and master-key provisioning required by later backend work.

Do not rerun or redesign this task unless a later Sol review creates a narrowly scoped migration/fix task.

---

## Task 007 — Authentication Foundation — COMPLETE

**Committed baseline:** `bd7687d5cdefff2470f5c1753e2fef740e2d2a18`

Established administrator password provisioning, Argon2id verification, session/CSRF storage and HTTP authentication boundaries. The external Sol security gate-fix review records the gate blockers as fixed before the committed baseline was accepted.

Do not add remote bootstrap/admin-create behavior without a new product/security decision.

---

## Task 008 — Protocol Credential Foundation — COMPLETE

**Committed baseline:** `46c7423c777fb9e2aa739f008113410ab8d4a853`

Established typed Phase 1 protocol configuration/credential schemas, strict canonical encoding/validation, and authenticated credential encryption. This task intentionally did not add Node API, direct node mutation, live Sing-box config orchestration, or service control.

---

## Task 009A — Config Transaction Filesystem Primitives — COMPLETE

**Committed baseline:** `46ebffcd7ffb5791e5c3c7ac39edf01473d24806`

Established the trusted filesystem primitives for the shared Sing-box config transaction, including cross-process locking, bounded config snapshots, candidate/backup artifacts, strict durable journal, atomic replacement, cleanup, retention, and the reviewed LockGuard operation-lifetime fix.

Task 009A does not execute Sing-box, control services, commit application state, expose HTTP, or orchestrate the complete production transaction.

---

## Task 009B — Config Transaction Coordinator and Recovery Core

**Owner:** Luna

### Goal

Implement the orchestration and crash-recovery decision layer for the shared Sing-box config transaction using the existing Task 009A filesystem primitives.

Task 009B turns the safe low-level primitives into one deterministic transaction state machine. External Sing-box checking, service-state control, and application-state commit must be represented by narrow injected interfaces/fakes so the failure and recovery rules can be tested before real OS command adapters are introduced.

### Allowed Scope

Primary scope:

- `backend/internal/singbox/configtx/**`

Small supporting interfaces/types may be added under:

- `backend/internal/singbox/**`

only when required to keep the coordinator independent of concrete OS/service/database implementations.

### Do Not Change

- no frontend changes
- no HTTP/API Node endpoints
- no public subscription endpoint
- no new database migration
- no direct node CRUD implementation
- no real `systemctl`, `rc-service`, OpenRC, or arbitrary service command execution
- no generic command execution layer
- no real Sing-box deployment flow
- no TLS import flow
- no Rotate endpoint
- no Refresh/Rotate semantic changes
- no `listen_port` / `public_port` model changes

### Contract / Behavior References

- `AGENTS.md` — Sing-box Config Safety and lock lifetime
- `docs/ARCHITECTURE.md` — Config Manager responsibilities
- `docs/SINGBOX_RULES.md` — Config Transaction, Crash Recovery Decision, Concurrency, Service Management
- `docs/SECURITY.md` — command/path/secret boundaries
- committed Task 009A primitives in `backend/internal/singbox/configtx`

### Required External Boundaries

Keep interfaces narrow and behavior-oriented. Exact names may differ, but tests must be able to inject:

- a candidate checker that validates the candidate before live replacement;
- a service state/controller boundary that reads prior active/stopped state, restores intended state, and verifies it;
- an application-state/generation boundary that reads committed `config_generation` and atomically commits the candidate application state with the matching subscription snapshot.

Task 009B uses fakes for these boundaries. Production `sing-box check`, Systemd/OpenRC, Node storage mutation, and subscription publication adapters are later tasks.

### Required Apply Flow

One cross-process config lock remains held across the complete mutation:

1. recover or reject an existing durable transaction before a new mutation;
2. snapshot the current live config;
3. read the old application generation;
4. snapshot the prior service state;
5. create a trusted transaction ID;
6. write the durable candidate artifact;
7. run the injected candidate checker;
8. create the durable backup;
9. create the `prepared` journal with old/candidate hashes, generations, service state, and trusted metadata;
10. atomically install the candidate;
11. restore the intended prior service state and verify candidate runtime state;
12. commit candidate application state + matching subscription snapshot through the injected commit boundary;
13. only after commit succeeds, finish/remove the journal and transaction artifacts safely;
14. apply bounded backup retention.

The lock must remain held through the application/subscription commit and unambiguous transaction cleanup.

### Failure Semantics

Before live replacement:

- live config remains unchanged;
- application generation remains old;
- no service-state mutation leaks from the failed attempt;
- safe temporary/candidate cleanup may run.

After live replacement but before application commit, any failure — including service verification or application commit failure — must trigger rollback:

1. restore the old config from the trusted backup;
2. restore the old service state;
3. verify the old working state;
4. leave application generation at the old value;
5. remove the journal only when rollback is known-good.

Rollback failure is a distinct critical state:

- return a stable typed rollback-failed error;
- durably mark the journal `rollback_failed`;
- preserve journal, backup, and candidate evidence;
- reject later mutations while that journal remains;
- never guess or silently clean up.

### Crash Recovery Decision

For a durable `prepared` journal, SQLite/application generation is the commit decision and file/artifact hashes must be verified before acting.

If current generation equals `old_generation`:

- treat the transaction as uncommitted;
- restore old config and old service state;
- verify old state;
- cleanup only after successful recovery.

If current generation equals `candidate_generation`:

- treat the application commit as authoritative;
- ensure the trusted candidate config is live;
- restore intended service state;
- verify candidate state;
- cleanup only after successful recovery.

If the generation, live hash, backup, or candidate evidence matches neither valid side:

- quarantine mutations;
- return a stable operator-action error;
- preserve journal/artifacts;
- never guess which side won.

A `rollback_failed` journal must block automatic mutation/retry and require operator action.

### Concurrency Requirements

- acquire the same Task 009A cross-process lock;
- concurrent config mutations serialize or fail cleanly;
- never commit application/subscription state after releasing the config lock;
- no service mutation occurs outside the lock during the transaction;
- cancellation must not abandon required rollback or critical-state preservation after live apply.

### Security Requirements

- journal/error/log text must not contain raw config, credentials, passwords, private keys, subscription tokens, or candidate bytes;
- no user-controlled shell command construction;
- no caller-controlled arbitrary filesystem paths;
- preserve Task 009A no-follow, ownership, mode, and bounded-read rules;
- distinguish ordinary transaction failure, quarantine/inconsistent recovery state, and rollback failure without leaking secrets.

### Acceptance Criteria

- one coordinator owns the complete apply/recovery state machine;
- existing Task 009A primitives are reused rather than bypassed;
- `prepared` journal is durable before live replacement;
- application/subscription commit occurs while the same config lock is held;
- application commit failure rolls back live config/service;
- rollback failure is durable and blocks subsequent mutation;
- old-generation and candidate-generation crash recovery converge correctly;
- ambiguous recovery state quarantines rather than guessing;
- a service that was stopped before the transaction remains stopped after a successful update;
- Task 009B still contains no production Systemd/OpenRC execution, HTTP Node API, or database schema change.

### Required Test Matrix

At minimum cover deterministic failure injection for:

- successful apply while prior service was running;
- successful apply while prior service was stopped;
- candidate checker failure before live mutation;
- backup/journal preparation failure;
- atomic install failure;
- service restore/restart failure after install;
- candidate runtime verification failure;
- application/SQLite commit failure after live apply with successful rollback;
- rollback config-restore failure with `rollback_failed` evidence preserved;
- rollback service restore/verify failure with `rollback_failed` evidence preserved;
- recovery with old generation;
- recovery with candidate generation;
- mismatched generation quarantine;
- mismatched old/candidate config hash/artifact quarantine;
- `rollback_failed` recovery blocks mutation;
- concurrent mutation cannot cross the active lock boundary;
- cancellation before mutation;
- cancellation after live apply still performs required rollback/critical-state preservation;
- no secret material appears in journal or returned error text.

### Verification

From `backend/` run fresh:

- `gofmt -l $(rg --files -g '*.go')`
- `go mod tidy`
- `go test -count=1 ./...`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go vet ./...`
- `go build -o /tmp/subbox-task009b/subbox ./cmd/subbox`
- `git diff --check`
- `git status --short -- frontend`

The frontend status must remain empty.

Also grep production source to confirm Task 009B did not introduce concrete `systemctl`, `rc-service`, arbitrary command execution, HTTP Node APIs, or secret logging.

### Review Gate

Stop after Task 009B implementation and verification.

Do not begin real Systemd/OpenRC adapters, production `sing-box check`, Node API/storage mutation, subscription publication endpoints, or Rotate until Sol reviews the coordinator and recovery behavior.
