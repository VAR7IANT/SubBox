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
