# gix-server

Backend único e multi-canal do [gix](https://github.com/Pedro-0101/gix). É o
**cérebro**: concentra toda a lógica de negócio (captura de notas, vetorização,
busca híbrida, IA, alertas e agendamento) atrás de um **core agnóstico de canal**.

Desktop, WhatsApp, Telegram, app Android — todos são **canais finos** sobre a
mesma API, com a **mesma fonte de dados** e a **mesma resposta**. Os canais não
têm lógica de negócio; só traduzem entrada/saída.

## Arquitetura

```
canais            adapters            core (agnóstico)        dados
─────────         ─────────           ────────────────        ─────
desktop  ─┐
whatsapp ─┤── REST/SSE/webhook ──►  intents (capture,    ──►  Postgres
telegram ─┤                          find, ask, chat,          + pgvector
android  ─┘                          create_alert, ...)        + tsvector (FTS)
                                          │
                                          └─► relay de IA (OpenRouter, chave no servidor)
```

- **`internal/core`** — o contrato. Tipos de domínio (`types.go`) e as intents
  (`core.go`): `Notes`, `Alerts`, `Chat`, `History`, agrupadas em `core.Core`.
  Toda intent é escopada por `userID`; o core sempre filtra por usuário, nunca
  confia no canal pra isolar dados.
- **adapters de canal** (fase 1+) — REST + SSE para desktop/web; webhooks para
  bots. Traduzem canal ⇄ intents.
- **dados** — Postgres com `pgvector` (embeddings e5-small, 384 dims) e `tsvector`
  (FTS). Ver `migrations/0001_init.sql`.

### Decisões

- **Multi-user**: contas + JWT; dados isolados por `user_id`.
- **IA via relay**: a chave da OpenRouter vive só no servidor.
- **Vetorização + busca no servidor**: embeddings e busca híbrida (FTS + pgvector,
  fundidos por RRF) são server-side. Reusa a stack ONNX do gix (`onnxruntime_go`
  via purego, sem CGO, roda em Linux).
- **Scheduler central + push**: o servidor agenda e dispara alertas; no vencimento
  faz push pro canal preferido do usuário (funciona com o desktop fechado).
- **Streaming**: SSE para desktop/web; bots recebem a resposta completa. O core
  emite `ChatEvent` de forma agnóstica via `ChatSink`.

## Status

**Fase 0 (fundação & contrato)** — feito: módulo, `internal/core` (contrato de
tipos + intents), schema Postgres (`migrations/0001_init.sql`), esqueleto HTTP.

**Fase 1 (camada de dados completa)** — funciona ponta a ponta (testado contra Postgres+pgvector):
- `config` (env/.env) → `store` (pgx + migrations embutidas) → `service`
  (implementa o core) → `httpapi` (REST + auth Bearer).
- Auth: signup/login com bcrypt + JWT; refresh token opaco (hash sha256, rotação
  a cada uso) via `POST /v1/auth/refresh`; middleware injeta `userID`; dados
  isolados por usuário (verificado: outro usuário leva 404 em recurso alheio).
- Notas CRUD + grafo: `GET/POST /v1/notes`, `GET/PUT/DELETE /v1/notes/{id}`,
  `PUT /v1/notes/{id}/char-limit`, `GET /v1/notes/graph`.
- Alertas CRUD: `GET/POST /v1/alerts`, `POST /v1/alerts/{id}/done|cancel|snooze`
  (criação por linguagem natural fica p/ a IA; este `POST` cria a partir de
  lembrete já estruturado).
- Histórico: `GET /v1/conversations`, `GET /v1/conversations/{id}/messages`,
  `DELETE /v1/conversations/{id}`.
- `Dockerfile` (binário estático, sem CGO) pronto pra Railway.

Falta (fase 2+): embeddings + busca híbrida, relay de IA + SSE, scheduler+push, e
o deploy na Railway. As intents de IA (Capture/Find/Ask/Summarize/Tidy, Chat,
Alerts.Create por linguagem natural) satisfazem o contrato mas retornam 501.
Roteiro completo: `docs/todo` no repo `gix`.

## Rodar

```sh
cp .env.example .env   # preencha DATABASE_URL, JWT_SECRET, OPENROUTER_API_KEY
go run ./cmd/server    # sobe em :8080 (ou $PORT); aplica as migrations no boot
curl localhost:8080/healthz

# Postgres local com pgvector p/ dev:
docker run -d --name gix-pg -e POSTGRES_PASSWORD=gix -e POSTGRES_DB=gix \
  -p 55432:5432 pgvector/pgvector:pg16
# DATABASE_URL=postgres://postgres:gix@localhost:55432/gix?sslmode=disable
```
