# Próximas Etapas — Handoff para Agente

Ordem de execução: **E → B → D → C**

Cada etapa é independente. Faça uma de cada vez, rodando `go build ./...` e
`go vet ./...` no final de cada uma.

---

## Etapa E — Paginação + Refinamentos CRUD

**O que:** endpoints de listagem sem limite (`ListNotes`, `ListAlerts`,
`ListConversations`) são perigosos com muitos registros. Adicionar paginação.

### Store (`internal/store/`)

Adicionar parâmetros de paginação em **todos os métodos de listagem**:

```go
type Pagination struct {
    Limit  int
    Offset int
}

func DefaultPagination() Pagination {
    return Pagination{Limit: 50, Offset: 0}
}
```

Alterar assinaturas (ou criar versões paginadas):

```go
func (s *Store) ListNotes(ctx context.Context, userID int64, p Pagination) ([]core.Note, error)
func (s *Store) ListAlerts(ctx context.Context, userID int64, p Pagination) ([]core.Alerts, error)
func (s *Store) ListConversations(ctx context.Context, userID int64, p Pagination) ([]core.Conversation, error)
```

Adicionar `LIMIT $3 OFFSET $4` em cada query. Valor padrão: limit=50, offset=0.
Validar: limit máximo 200, mínimo 1; offset mínimo 0.

### Core (`internal/core/types.go` ou `core.go`)

Não precisa mexer nas interfaces — a paginação é detalhe de implementação do
store. Mas considere adicionar um tipo `Page[T any]` se quiser retornar
total-count:

```go
type Page[T any] struct {
    Items []T `json:"items"`
    Total int `json:"total"`
}
```

Para esta etapa, manter simples: só `limit` + `offset` nos parâmetros de query
string, sem total-count (evita COUNT extra).

### Service (`internal/service/`)

Os métodos do service (`List`, etc.) atualmente delegam direto ao store. Passar
`Pagination` como parâmetro ou extrair de um struct de input.

Simplifique: criar `ListOptions` no service que encapsula paginação:

```go
type ListOptions struct {
    Limit  int
    Offset int
}

func parsePagination(limit, offset int) ListOptions {
    if limit <= 0 || limit > 200 {
        limit = 50
    }
    if offset < 0 {
        offset = 0
    }
    return ListOptions{Limit: limit, Offset: offset}
}
```

### HTTP (`internal/httpapi/`)

Ler `?limit=` e `?offset=` da query string em **cada handler de listagem**:

```go
func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
    userID, _ := auth.UserID(r.Context())
    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
    opts := service.ParseListOptions(limit, offset)
    ...
}
```

Arquivos a modificar:
- `internal/store/notes.go` — `ListNotes` ganha `Pagination`
- `internal/store/alerts.go` — `ListAlerts` ganha `Pagination`
- `internal/store/conversations.go` — `ListConversations` ganha `Pagination`
- `internal/service/` — se houver wrapper, adicionar `ListOptions`
- `internal/httpapi/notes.go` — `listNotes` lê query params
- `internal/httpapi/alerts.go` — `listAlerts` lê query params
- Qualquer handler de listagem de conversas

### Verificação

```bash
go build ./... ; go vet ./... ; Write-Host "build=$?" ; Write-Host "vet=$?"
```

---

## Etapa B — Preferências por Usuário

**O que:** `user_prefs` já existe no banco (model, language, system_prompt,
note_char_limit) mas o server usa valores fixos. Carregar e aplicar.

### Store

Adicionar em `internal/store/users.go` (ou novo `store/prefs.go`):

```go
type UserPrefs struct {
    Model        string `json:"model"`
    Language     string `json:"language"`
    SystemPrompt string `json:"systemPrompt"`
    CharLimit    int    `json:"charLimit"`
    Timezone     string `json:"timezone"`
}

func (s *Store) GetUserPrefs(ctx context.Context, userID int64) (UserPrefs, error)
// SELECT model, language, system_prompt, note_char_limit, COALESCE(timezone,'UTC')
// FROM user_prefs WHERE user_id = $1
// Se não existir, devolver UserPrefs{} (valores default)
```

Adicionar setters:

```go
func (s *Store) SetUserPrefs(ctx context.Context, userID int64, p UserPrefs) error
// UPSERT: INSERT ... ON CONFLICT (user_id) DO UPDATE
```

### Service

**Onde usar as preferências:**

| Intent | Campo que usa |
|--------|---------------|
| `Chat.Send` | `model`, `language`, `system_prompt` |
| `Notes.Capture` | `language` |
| `Notes.Ask` | `language` |
| `Notes.Summarize` | `language` |
| `Notes.Tidy` | `language` |
| `Alerts.Create/CreateForNote` | `timezone` (já implementado) |
| `CreateFromProposal` / `AppendTo` | `char_limit` (fazer overflow check) |

**Estratégia:** cada método de intent aceita `userID` e carrega `UserPrefs`
quando necessário. Não pré-carregar em todo lugar — só onde os campos são
usados.

**Remover `defaultLanguage`** de `service/ai.go`. Substituir por parâmetro nos
prompts. Onde o idioma era fixo, passar `prefs.Language` (se vazio, `"pt"`).

No `Chat.Send`, carregar `prefs.Model` para escolher o modelo de IA (fallback
para `config.DefaultModel` se vazio).

### HTTP

Adicionar em `internal/httpapi/` (novo arquivo `prefs.go`):

```go
func (s *Server) getPrefs(w http.ResponseWriter, r *http.Request)
// GET /v1/prefs → retorna UserPrefs

func (s *Server) updatePrefs(w http.ResponseWriter, r *http.Request)
// PUT /v1/prefs → atualiza campos enviados (merge parcial)
```

Registrar rotas em `server.go`:
```go
mux.Handle("GET /v1/prefs", protected(s.getPrefs))
mux.Handle("PUT /v1/prefs", protected(s.updatePrefs))
```

**Merge parcial:** no update, só sobrescrever campos que vieram no JSON. Os
demais mantêm o valor atual.

### Verificação

```bash
go build ./... ; go vet ./... ; Write-Host "build=$?" ; Write-Host "vet=$?"
```

---

## Etapa D — Rate Limiting + Hardening

### Rate limiting por usuário

Criar `internal/ratelimit/`:

```go
package ratelimit

import (
    "sync"
    "time"
)

type Store struct {
    mu    sync.Mutex
    users map[int64]*bucket
}

type bucket struct {
    tokens   int
    lastTick time.Time
}

func New() *Store { return &Store{users: make(map[int64]*bucket)} }

// Allow verifica se o usuário pode passar. rate = requisições por segundo,
// burst = máximo acumulado.
func (s *Store) Allow(userID int64, rate, burst int) bool
    // Implementar token bucket simples
```

Parâmetros default: rate=10, burst=20 (10 req/s, pico de 20). Para rotas de
streaming (chat, push), rate menor (rate=2, burst=5) porque são conexões longas.

**Como integrar:**

Em `internal/httpapi/server.go`, criar middleware:

```go
func (s *Server) rateLimit(rate, burst int) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userID, found := auth.UserID(r.Context())
            if !found {
                next.ServeHTTP(w, r)
                return
            }
            if !rl.Allow(userID, rate, burst) {
                http.Error(w, "muitas requisições", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Aplicar o middleware nas rotas protegidas. Rotas de streaming (chat, push) usam
rate menor. Rotas normais (CRUD) usam rate padrão.

### Logs estruturados

Trocar `log.Printf`/`fmt.Printf` por `slog` (stdlib, Go 1.21+).

Arquivos com logging atualmente:
- `cmd/server/main.go` — logs de boot
- `internal/scheduler/scheduler.go` — logs de disparo
- `internal/embed/` — logs de download

Para logs de request, adicionar middleware HTTP em `server.go`:

```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "duration", time.Since(start))
    })
}
```

### Verificação

```bash
go build ./... ; go vet ./... ; Write-Host "build=$?" ; Write-Host "vet=$?"
```

---

## Etapa C — Cobertura de Testes

**O que:** hoje só `internal/ai/client_test.go` tem testes (unitários, mockam
HTTP). Faltam testes de store (com Postgres real ou testcontainers) e de service
(com AI mockado).

### Testes de Store (integração com Postgres)

Criar `internal/store/store_test.go`:

```go
package store_test

// Usar testcontainers-go para levantar Postgres + pgvector:
// https://github.com/testcontainers/testcontainers-go

import (
    "context"
    "testing"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupStore(t *testing.T) (*Store, func()) {
    ctx := context.Background()
    pg, err := postgres.Run(ctx,
        "pgvector/pgvector:0.7.0-pg16",
        postgres.WithDatabase("gix_test"),
    )
    // ... obter conexão, rodar migrations, retornar store + cleanup
}
```

Testes a escrever (1 arquivo por domínio):

- `store/notes_test.go` — CRUD, FTS search, vector search, NotesByIDs
- `store/alerts_test.go` — CRUD, DueAlerts, SetAlertStatus
- `store/conversations_test.go` — CRUD, GetMessages, DeleteConversation (cascade)
- `store/users_test.go` — CreateUser, UserByEmail, UserTimezone

**Padrão de cada teste:**

```go
func TestCreateNote(t *testing.T) {
    s, cleanup := setupStore(t)
    defer cleanup()
    note, err := s.CreateNote(ctx, userID, "title", "content", []string{"tag1"}, 0)
    assert.NoError(t, err)
    assert.NotZero(t, note.ID)
    assert.Equal(t, "title", note.Title)
}
```

**Dica:** se testcontainers for pesado demais, criar um `store_test.go` que
pula (`t.Skip("requires Postgres")`) quando `DATABASE_URL` não está setada, e
usa uma conexão real quando está. Quem quiser rodar testes de integração sobe
um Postgres local.

### Testes de Service (com AI mockado)

Criar `internal/service/service_test.go`:

Mock para AI:

```go
type mockAI struct {
    completeFunc func(ctx context.Context, msgs []ai.Message) (string, core.Usage, error)
}

func (m *mockAI) Complete(...) { ... }
```

Testes:

- `TestCapture_CreatesNote` — AI devolve JSON válido → nota criada
- `TestCapture_AttachProposed` — AI escolhe attach_to → status attach_proposed
- `TestCapture_NoAPIKey` — sem chave → status no_api_key
- `TestAsk_ReturnsSummary` — Find + AI → AskResult com summary
- `TestChatSend_EmitsDelta` — StreamTools mockado → sink recebe deltas
- `TestChatSend_ToolCallNote` — AI faz tool_call → sink recebe note_proposed

### Testes de HTTP (com httptest)

Criar `internal/httpapi/httpapi_test.go`:

Usar `httptest.NewServer` + mock core:

```go
func TestListNotes(t *testing.T) {
    core := &mockCore{notes: &mockNotes{...}}
    srv := New(core, auth, store, pushHub)
    ts := httptest.NewServer(srv)
    defer ts.Close()
    
    resp, _ := http.Get(ts.URL + "/v1/notes")
    assert.Equal(t, 200, resp.StatusCode)
}
```

### Rodando

```bash
# Unitários (sem Postgres)
go test ./internal/ai ./internal/config ./internal/ratelimit

# Com Postgres (se DATABASE_URL estiver setada)
go test ./internal/store -run TestStore

# Todos
go test ./... -short
```

### Verificação

```bash
go build ./... ; go vet ./... ; Write-Host "build=$?" ; Write-Host "vet=$?"
go test ./internal/ai ./internal/config
```

---

## Resumo dos arquivos a tocar por etapa

| Etapa | Novos | Editados |
|-------|-------|----------|
| **E** | — | `store/notes.go`, `store/alerts.go`, `store/conversations.go`, `httpapi/notes.go`, `httpapi/alerts.go`, `httpapi/server.go` |
| **B** | `httpapi/prefs.go` | `service/ai.go`, `service/notes.go`, `service/stubs.go`, `store/users.go`, `httpapi/server.go`, `service/notes_prompts.go` |
| **D** | `internal/ratelimit/ratelimit.go` | `httpapi/server.go`, `cmd/server/main.go`, `internal/scheduler/scheduler.go` |
| **C** | Vários `*_test.go` | — |

Faça uma etapa de cada vez, na ordem E → B → D → C. No final de cada etapa
rode `go build ./...` e `go vet ./...` antes de passar para a próxima.
