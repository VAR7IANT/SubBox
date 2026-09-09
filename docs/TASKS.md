# Tasks

This file is the execution queue. Sol owns task decomposition and review. Luna implements one task at a time.

## Task 001 — Frontend Foundation

**Owner:** Luna

### Goal

Create the initial frontend project using React + TypeScript + Vite + Tailwind CSS. Prepare a clean project structure that can support shadcn/ui components later.

### Allowed Scope

- `frontend/**`
- root package/tooling files only when strictly required for the frontend bootstrap

### Do Not Change

- API contract
- data model
- backend architecture
- product scope

### Acceptance Criteria

- frontend builds successfully
- TypeScript strict mode enabled
- basic app shell renders
- no monitoring/dashboard-chart dependencies added

### Verification

- install dependencies
- build
- typecheck
- lint if configured

---

## Task 002 — App Shell and Sidebar

**Owner:** Luna

### Goal

Implement the static application shell based on `docs/ui/subbox-ui-orange.png`.

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

### Do Not Change

No backend/API work.

### Acceptance Criteria

- layout visually follows the reference
- no CPU/RAM/disk/ping/traffic monitoring UI
- responsive enough not to depend on one exact screenshot width

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

### Behavioral UI Rule

Refresh and Rotate must look and behave as distinct actions. Rotate requires confirmation in the UI flow.

### Acceptance Criteria

- copy interaction works in browser
- QR dialog can open using mock subscription URL
- Refresh has loading/success mock state
- Rotate confirmation dialog exists

---

## Task 004 — Node Cards and NAT Port UI

**Owner:** Luna

### Goal

Implement reusable static/mock node cards for:

- VLESS Reality
- Hysteria2
- AnyTLS

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

### Acceptance Criteria

The UI clearly communicates that `public_port` may differ from `listen_port`.

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

### Important

Changing only `public_port` must not imply that `listen_port` changed.

### Acceptance Criteria

- validation errors are visible
- save/cancel states work
- successful mock update refreshes the node card
- no real backend call yet unless Sol has explicitly updated this task

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
