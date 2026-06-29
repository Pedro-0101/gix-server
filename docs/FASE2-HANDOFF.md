# Fase 2 (IA + busca) — FINALIZADA ✅

Todas as intents de IA foram implementadas. Nenhuma retorna `core.ErrNotImplemented`.

## ✅ Feito

### Relay de IA
- `internal/ai/client.go` — `Complete`, `Stream`, `StreamTools`, `HasKey`.

### Embeddings server-side (ONNX e5-small)
- `internal/embed/` portado do `gix` (modelo ONNX, mean-pool + L2 normalize, 384-dim).
- Gerado ao criar/editar nota. Job de backfill para notas antigas.

### Busca híbrida RRF
- `Notes.Find`: FTS (`tsvector` + `unaccent`) + pgvector (cosseno) + RRF (k=60).
- `Notes.Ask`: Find top-N + resumo IA.

### Summarize / Tidy
- `Notes.Summarize`: IA condensa nota em Markdown.
- `Notes.Tidy`: IA reorganiza preservando toda a informação.

### Notes.Capture
- `POST /v1/notes/capture` — IA estrutura texto livre em nota.
- Roteamento: `candidateNotes` (vetorial, limiar 0.82) → attach vs criar.
- Alerta opcional detectado no texto.

### Notes.ResolveOverflow
- Modos: `part2` (irmã), `summarize` (mescla + IA condensa), `split` (mescla + IA divide).

### Alerts.Create / CreateForNote
- `POST /v1/alerts/parse` — parsing de linguagem natural via IA.
- `POST /v1/notes/{id}/alert` — alerta vinculado a nota.

### Chat.Send com SSE
- `POST /v1/chat` — SSE streaming com eventos: `delta | done | error | usage | note_proposed | alert_proposed`.
- Tool-calls: `create_note` e `create_alert` (propose-then-confirm).
- Persistência de conversas + mensagens no Postgres.

## Arquivos tocados nesta fase

```
internal/ai/client.go + client_test.go
internal/config/pricing.go (novo)
internal/service/ai.go (novo)
internal/service/notes_prompts.go (novo)
internal/service/stubs.go → Chat.Send implementado
internal/httpapi/chat.go (novo)
internal/service/notes.go → Capture + ResolveOverflow
internal/store/conversations.go → CreateConversation + AddMessage
internal/httpapi/notes.go → +captureNote handler
internal/httpapi/server.go → +rotas capture + chat
docs/ROADMAP.md → atualizado
docs/FASE2-HANDOFF.md (este)
```
