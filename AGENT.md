# AGENT.md

Instructions for any agent or contributor working in this repository. This is
the source of truth; `CLAUDE.md` points here.

## Project

`gix` is a Spotlight/Raycast-style AI chat overlay for the desktop: a frameless
window summoned by a global hotkey that streams chat from OpenRouter, captures
notes, runs alerts, and tracks token cost. Backend is **Go + Wails v3**
(AI client, config, SQLite, embeddings — all pure Go); frontend is **React +
TypeScript + Tailwind v4** embedded as web assets. See `README.md` for the user
guide and full directory map.

## Build / test / run

- **Run / dev:** `wails3 dev` (hot reload). Build: `wails3 build`. Installer:
  `wails3 package`. Requires the Wails v3 CLI on PATH (`wails3 doctor`).
- **Go tests:** require `CGO_ENABLED=1` and an msys2 gcc on PATH (sqlite/ONNX).
  Run `CGO_ENABLED=1 go test ./...`. Format with `gofmt`, vet with `go vet ./...`.
- **Frontend tests:** `npm test` (watch) / `npm run test:run` (CI), Vitest,
  pure-logic tests in `src/**/*.test.ts`.
- **Frontend quality:** `npm run lint` (ESLint), `npm run format` (Prettier),
  `npm run check:lines` (file-size guard). Run them before committing.

## File-size limits

- **≤100 lines** for simple files; **≤300 lines** for complex ones. Always keep
  to the minimum that stays clear — small is the goal, the limits are ceilings.
- Data-only files are exempt (e.g. `frontend/src/i18n.ts` translation dicts,
  the model-pricing table in `internal/config/config.go`).
- When a file crosses a limit, **split by responsibility**, not by line count —
  each new file should own one coherent concern. Don't just move lines around.
- `npm run check:lines` enforces the 300-line ceiling for source files.

## SOLID

Follow SOLID as far as it's practical:

- **Single responsibility:** one reason to change per file/type/function.
- **Open/closed:** extend via new registrations, not by editing dispatchers (see
  the command registry below).
- **Dependency inversion:** depend on interfaces, not concretes. The Go services
  already do this — `Streamer`, `Completer`, `Embedder`, `notifier`, `Emitter`
  in `internal/app/*.go` are injected via constructors. Follow that style.

## Design patterns (house style)

Reach for a pattern when it makes SOLID natural — match what already exists:

- **Constructor DI + interface fakes (Go):** services take their deps as
  constructor args; tests pass fakes (see `internal/app/shell.go`,
  `*_test.go`).
- **Command registry + context-as-DI (frontend):** add commands to the array in
  `frontend/src/commands/registry.ts`; they act only through `CommandContext`
  (`commands/types.ts`), never reaching into React state directly.
- **Strategy / adapter at boundaries:** the `internal/ai`, `internal/embed`, and
  `internal/hotkey` packages are swappable implementations behind app-level
  interfaces. Keep platform/vendor specifics there.

## Testing

- **Go:** table-driven tests with interface fakes, temp DBs via `t.TempDir()`.
  No mocking libraries.
- **Frontend:** Vitest, pure logic extracted from components so it tests without
  a DOM (see `commands/interaction.ts`, `commands/highlight.ts`).
- New logic ships with tests. Keep behavior-preserving refactors green.

## Conventions

- **Never hand-edit `frontend/bindings/**`** — Wails generates it.
- Use design tokens (CSS variables in `frontend/src/styles/tokens.css`); no
  literal colors in components.
- Commits: Conventional Commits in Portuguese, as in history
  (`fix(note): ...`, `chore: ...`).

## Cleanup roadmap

Stages 1–5 below are **done** — every source file is now under the 300-line
ceiling (`npm run check:lines` passes). They're kept as a record of how the
split was organized; follow the same pattern (split by responsibility, keep
tests green, behavior identical) for future growth.

1. ✅ **`internal/db/db.go`** — split into `db.go` (connection), `schema.go`,
   `notes.go`, `search.go`, `alerts.go`, `conversations.go`.
2. ✅ **`internal/app/notes.go`** — split into `notes.go` (service + CRUD +
   shared helpers), `notes_capture.go`, `notes_search.go` (hybrid FTS+vector+RRF),
   `notes_ask.go`, `notes_graph.go`.
3. ✅ **`internal/app/alerts.go`** — split into `alerts.go` (service +
   management), `alerts_create.go` (parsing/store), `alerts_scheduler.go` (poll
   loop + toast dispatch); recurrence stays in `recurrence.go`.
4. ✅ **`frontend/src/App.tsx`** — extracted hooks (`lib/useChat`,
   `useInteraction`, `useWindowFit`, `useCommandContext`) and components
   (`InputBar`, `ShellPanel`, `icons`); App is now orchestration only.
5. ✅ **`GraphView.tsx` / `NotesView.tsx`** — graph simulation/rendering moved to
   `views/graph/{simulation,render,types}.ts`; the reusable `UndoToast` extracted.
6. **Sweep (ongoing)** — keep near-limit files tidy as they grow: `shell.go`
   (239), `internal/ai/client.go` (226), `internal/embed/{lib,embed}.go` (~225).
   Test files (`notes_test.go`, etc.) are exempt from the ceiling.
