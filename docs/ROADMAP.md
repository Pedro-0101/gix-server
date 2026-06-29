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
- Auth (bcrypt + JWT + middleware), notas CRUD + grafo, isolamento por usuário.
- Migration `0001_init.sql` com pgvector, FTS (`tsvector` via wrapper `IMMUTABLE`
  + `unaccent`), e tabelas de notes/alerts/conversations/messages/users/user_prefs.
- Intents de IA e Alerts/Chat/History satisfazem o contrato mas retornam
  `core.ErrNotImplemented` (→ 501).

---

## Fase 1 (restante) — completar a camada de dados

Portar o resto do `internal/db` do `gix` para o `store`, sem IA. Padrão já
estabelecido em `internal/store/notes.go` (queries escopadas por `user_id`,
mapear `pgx.ErrNoRows` → `core.ErrNotFound`, slices nunca nil).

- **`store/alerts.go`** + implementar em `service/stubs.go` (mover `Alerts` p/ um
  arquivo próprio): `List`, `Cancel`, `Done`, `Snooze`, `CreateProposed` são CRUD
  puro (sem IA). `Create`/`CreateForNote` dependem de parsing por IA → deixar 501
  até a fase 2. Reusar a lógica de `recurrence.go` do `gix` ao avançar recorrência.
- **`store/conversations.go`** + implementar `History` (`List`/`Messages`/`Delete`):
  CRUD puro, portar de `internal/db/conversations.go`.
- **Rotas HTTP** correspondentes em `httpapi` (`alerts.go`, `history.go`),
  seguindo o padrão de `notes.go` (extrair `userID`/`{id}`, mapear erros).
- **Refresh token** no `auth` (hoje só access token de 24h): endpoint
  `/v1/auth/refresh` e rotação. Guardar refresh tokens (tabela ou JWT de longa
  duração com revogação).

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

## Fase 3 — scheduler central + push

- Tabela de canais/push por usuário (ex.: `user_channels(user_id, kind, address,
  push_token, preferred)`) — nova migration.
- Loop de scheduler server-side (goroutine) que faz poll de `alerts` pendentes
  vencidos (índice `idx_alerts_due` já existe), respeitando o fuso do usuário;
  avança recorrência (reusar `recurrence.go`) ou marca `done`.
- No disparo, **push** pro canal preferido: WebSocket/SSE para o desktop;
  mensagem para WhatsApp/Telegram (fase 4). O desktop, ao receber, mostra a
  notificação nativa.

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
