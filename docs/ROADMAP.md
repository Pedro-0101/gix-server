# Roadmap do gix-server

Este documento descreve **o que falta fazer e como**, partindo do que já existe.
É o guia de execução do backend; o roteiro de produto (visão geral, fases) também
está em `docs/todo` no repo `gix`.

## Princípio que rege tudo

O `internal/core` é **agnóstico de canal**. Toda lógica de negócio vive nas
intents (`core.Notes`, `core.Alerts`, `core.Chat`, `core.History`), recebendo/
devolvendo dados simples e sempre escopadas por `userID`. Adapters de canal
(HTTP/REST, SSE, webhooks de bots) só traduzem entrada/saída — **nunca** contêm
regra de negócio. Antes de adicionar qualquer coisa, pergunte: "isso é lógica
(vai pro core/service) ou tradução de canal (vai pro adapter)?"

## Estado atual (feito)

- `config → store (pgx) → service → httpapi` ligado e testado contra Postgres+pgvector.
- Auth (bcrypt + JWT + middleware + refresh token rotacionável), notas/alertas CRUD
  + grafo, histórico de conversas, isolamento por usuário.
- Migration `0001_init.sql` com pgvector, FTS (`tsvector` via wrapper `IMMUTABLE`
  + `unaccent`), e tabelas de notes/alerts/conversations/messages/users/user_prefs.
  Migration `0002_refresh_tokens.sql` (hash sha256, expiração, revogação).
- Camada de dados completa (fase 1): `store/alerts.go`, `store/conversations.go`,
  `store/tokens.go`; `service/alerts.go` + `service/history.go` saíram do stub.
- Scheduler central + push SSE (fase 3): `internal/scheduler`, `internal/recur`,
  outbox `alert_deliveries`, `GET /v1/push`. Migration `0003_scheduler_push.sql`
  (`user_prefs.timezone` + outbox).
- Intents que dependem de IA/embeddings (Capture/Find/Ask/Summarize/Tidy, Chat,
  `Alerts.Create`/`CreateForNote` por linguagem natural) ainda retornam
  `core.ErrNotImplemented` (→ 501) — é a fase 2.

---

## Fase 1 (restante) — completar a camada de dados ✓ FEITO

Portado o resto do `internal/db` do `gix` para o `store`, sem IA. Padrão de
`internal/store/notes.go` (queries escopadas por `user_id`, `pgx.ErrNoRows` →
`core.ErrNotFound`, slices nunca nil).

- ✓ **`store/alerts.go`** + `service/alerts.go`: `List`, `Cancel`, `Done`,
  `Snooze`, `CreateProposed` são CRUD puro. `Create`/`CreateForNote` dependem de
  parsing por IA → seguem 501 até a fase 2. (Avançar recorrência via
  `recurrence.go` do `gix` entra no scheduler, fase 3.)
- ✓ **`store/conversations.go`** + `service/history.go` (`List`/`Messages`/`Delete`).
- ✓ **Rotas HTTP** em `httpapi` (`alerts.go`, `history.go`), padrão de `notes.go`.
- ✓ **Refresh token**: `store/tokens.go` + `auth.NewRefreshToken`/`HashRefreshToken`,
  endpoint `POST /v1/auth/refresh` com rotação (token opaco, guardado por hash
  sha256, revogado a cada uso). Migration `0002_refresh_tokens.sql`.

## Fase 2 — embeddings, busca híbrida e relay de IA (o coração)

Aqui as intents de IA saem do 501. **Toda IA roda no servidor**; a chave da
OpenRouter vem de `config.OpenRouterKey`.

### Relay de IA
- Portar `internal/ai/client.go` do `gix` para cá (ex.: `internal/ai/`). Ele já é
  só HTTP para a OpenRouter (`Complete`, `StreamTools`) — copiar quase 1:1.
- Injetar o client no `service` (assinaturas de `NewNotes`/`NewChat`/`NewAlerts`
  ganham o client + a chave). Portar os prompts de `notes_capture.go`,
  `notes_ask.go`, `prompt.go`, `recurrence.go`.
- Implementar: `Notes.Capture`, `Ask`, `Summarize`, `Tidy`, `ResolveOverflow`;
  `Alerts.Create`/`CreateForNote` (parsing de tempo).

### Embeddings (server-side)
- Portar `internal/embed/` do `gix` (ONNX e5-small). `onnxruntime_go` usa purego
  (sem CGO) e roda em Linux — mas a `libonnxruntime.so` precisa de glibc, então a
  imagem final do Docker deixa de ser `distroless/static`: trocar por
  `debian:bookworm-slim` e copiar/baixar a lib no build (ver nota no `Dockerfile`).
- Gerar embedding ao criar/editar nota (em `service`), gravando em `notes.embedding`
  (`vector(384)`). Usar `github.com/pgvector/pgvector-go` para o tipo na escrita/leitura.
- Embedar notas antigas (sem vetor) num job de backfill.

### Busca híbrida (RRF)
- Implementar `Notes.Find` portando `notes_search.go` do `gix`, mas server-side:
  - FTS: `WHERE fts @@ plainto_tsquery('portuguese', unaccent($q))` ordenado por
    `ts_rank`.
  - Vetor: `ORDER BY embedding <=> $queryVec` (cosseno, índice HNSW já existe).
  - Fundir as duas listas por **RRF** (k=60), como no `gix`. A query também passa
    por `unaccent` no lado FTS para casar o índice acento-insensível.
- `Ask` = `Find` + resumo por IA das top-N notas.

### Streaming (SSE)
- `Chat.Send` recebe um `core.ChatSink`; o adapter HTTP implementa o sink
  escrevendo eventos SSE (`text/event-stream`). Mapear `ChatEvent.Type`
  (delta/done/error/usage/note_proposed/alert_proposed) para `data:` lines.
- Tool-calls (`create_note`/`create_alert`) do chat viram `note_proposed`/
  `alert_proposed` no stream; o canal confirma e chama a intent CRUD correspondente.
- Bots (fase 4) ignoram o streaming: usam um sink que acumula e devolve o `done`.

## Fase 3 — scheduler central + push ✓ FEITO

- ✓ Loop de scheduler server-side (`internal/scheduler`, goroutine) com tick
  imediato no boot + poll a cada 30s sobre `idx_alerts_due`; avança recorrência
  (`internal/recur`, portado do `recurrence.go` do gix) no fuso do usuário
  (`user_prefs.timezone`, migration `0003`) ou marca `done`. `_ "time/tzdata"`
  no main p/ `LoadLocation` funcionar na imagem sem tzdata do SO.
- ✓ Push por **SSE** (`GET /v1/push`, `httpapi/PushHub` implementa
  `scheduler.Notifier`). Desacoplado por outbox (`alert_deliveries`): cada
  disparo persiste; entrega ao vivo marca `delivered_at`; cliente offline recebe
  as pendentes no reconnect (flush no connect). Nada se perde com o desktop
  fechado. O `Notifier` é o seam de transporte p/ os canais da fase 4.
- ⏳ `user_channels` (preferência de canal) adiada p/ a fase 4: com só o desktop
  (SSE) não há canal a preferir. Quando entrar WhatsApp/Telegram, o `Notifier`
  passa a rotear pelo canal preferido do usuário.
- Quando o desktop existir (fase 3 do `docs/todo` do gix): ao receber o evento
  SSE, mostra a notificação nativa.

## Fase 4 — segundo canal (prova do multi-canal)

- Adapter de WhatsApp ou Telegram em `internal/channels/<canal>/`: webhook de
  entrada → traduz para intents do core → resposta no formato do canal. **Sem
  duplicar lógica** — só tradução.
- Validar assinatura dos webhooks; ligar identidade do canal ↔ `user_id`
  (tabela de vínculo). Preferência de canal por usuário.

## Fase 5 — hardening & deploy

- Deploy na Railway: serviço a partir do `Dockerfile`, plugin de Postgres
  (pgvector), variáveis `DATABASE_URL`/`JWT_SECRET`/`OPENROUTER_API_KEY`.
- Rate limiting por usuário, logs estruturados, métricas de custo de IA por
  usuário, backups do Postgres.
- Migração de dados: importar o SQLite local do `gix` de cada usuário no primeiro
  login (script de upload de notas/alerts; embeddings são regerados no servidor).

---

## Convenções

- Queries sempre filtram por `user_id`; o core nunca confia no canal para isolar.
- Erros de domínio: `core.ErrNotFound` (→404), `core.ErrNotImplemented` (→501);
  o resto vira 500 em `httpapi/respond.go`.
- Contrato JSON em `camelCase` (tags em `internal/core/types.go`); listas vazias
  retornam `[]`, não `null`.
- Migrations idempotentes (`IF NOT EXISTS`), embutidas via `migrations/migrations.go`,
  aplicadas no boot. Novas migrations: `0002_*.sql`, `0003_*.sql`, …
- Sem CGO enquanto não houver ONNX; ao adicionar embeddings, ajustar o `Dockerfile`.
