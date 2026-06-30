# CLAUDE.md

All contributor and agent instructions for this repo live in
**[AGENT.md](./AGENT.md)** — read it before making changes. It covers the
file-size limits (≤100 simple / ≤300 complex), SOLID expectations, the
design-pattern house style, and the cleanup roadmap.

Claude Code reminders:

- Go tests need `CGO_ENABLED=1` and an msys2 gcc on PATH — run
  `CGO_ENABLED=1 go test ./...`.
- Before committing frontend changes: `npm run lint`, `npm run check:lines`,
  `npm run test:run`.
- Never hand-edit `frontend/bindings/**` (Wails-generated).
