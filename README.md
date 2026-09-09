# SubBox

SubBox is a lightweight web UI for Sing-box deployment, node management, NAT port mapping, and subscription delivery.

> Status: architecture and UI specification phase.

## Core goals

- Visual Sing-box deployment and node management
- Stable subscription URL
- Copy / QR / Refresh / Rotate workflows
- Independent `listen_port` and `public_port` for NAT / port forwarding
- Safe Sing-box config validation, apply, restart, verify, and rollback
- Support for Shadowsocks, Hysteria2, TUIC, VLESS Reality, and AnyTLS

## Explicitly out of scope

SubBox is not a server monitoring panel. It does not include a monitoring agent, telemetry collector, CPU/RAM/disk history, remote shell, web terminal, file manager, or arbitrary remote command execution.

## Technology direction

- Frontend: React + TypeScript + Vite + Tailwind CSS + shadcn/ui
- Backend: Go
- Storage: SQLite
- Sing-box config: `/etc/sing-box/config.json`
- SubBox state: `/etc/subbox/`

## Development docs

See `AGENTS.md` and the files under `docs/` before making architectural changes.

## Reference project

The deployment behavior is inspired by `caigouzi121380/singbox-deploy`, but SubBox is intended to be an independent implementation rather than a web wrapper around its shell scripts.
