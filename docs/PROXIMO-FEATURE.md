# Feature: Canal WhatsApp — Adapter do Multi-Canal

**Contexto:** O `gix-server` é um backend Go multi-canal. O `internal/core` é
agnóstico de canal. Já temos o canal HTTP/REST (fase 1) e push SSE (fase 3).
Agora vamos provar que a arquitetura multi-canal funciona criando um **adapter
de WhatsApp**.

## Princípio arquitetural (NÃO VIOLAR)

- `internal/core` contém as intents (`core.Notes`, `core.Alerts`, `core.Chat`,
  `core.History`). **Nunca** coloque lógica de negócio no adapter.
- O adapter só traduz: entrada do WhatsApp → chamada de intent → resposta
  formatada para WhatsApp.
- O mesmo `core.Core` é compartilhado entre todos os canais. Não crie um core
  separado para WhatsApp.

## O que implementar

### 1. Provider WhatsApp (`internal/channels/whatsapp/`)

Criar pacote `internal/channels/whatsapp/` com:

#### a) Client (envio de mensagens)
- Usar a API do **WhatsApp Cloud API** (Meta) — é a mais simples, via HTTP.
- `type Client struct` com `phoneNumberID`, `token` (bearer), `httpClient`.
- `SendText(ctx, to string, body string) error` — POST
  `https://graph.facebook.com/v22.0/{phoneNumberID}/messages` com:
  ```json
  {"messaging_product":"whatsapp","to":"{to}","type":"text","text":{"body":"{body}"}}
  ```
- `SendInteractive(ctx, to, body string, buttons []Button) error` — para
  confirmações (ex.: "Confirma criar nota?" com botões Sim/Não).
- Erros 4xx/5xx viram `fmt.Errorf("whatsapp: status %d: %s", code, body)`.

#### b) Webhook handler (recepção)
- `HandleWebhook(w http.ResponseWriter, r *http.Request)` — recebe mensagens
  do WhatsApp Cloud API.
- **Verificação do webhook**: `GET /webhook/whatsapp` — responde ao `hub.challenge`
  quando `hub.verify_token` casa com o token configurado (env `WHATSAPP_VERIFY_TOKEN`).
- **Mensagens recebidas**: `POST /webhook/whatsapp` — processa `entry[0].changes[0].value.messages[]`.
  - Extrair: `from` (telefone), `text.body` (conteúdo), `type`.
  - Ignorar `type != "text"` por enquanto.
  - Rotear para o handler de texto.

#### c) Identity resolver
- Tabela `user_channels` (ver migration abaixo) liga `phone` → `user_id`.
- Quando uma mensagem chega de um telefone desconhecido, responder com
  instruções de login (enviar `--login <email> <senha>` ou gerar token de uso
  único).
- Quando identificado, reusar o `userID` em todas as intents do core.

### 2. Roteador de texto WhatsApp (`internal/channels/whatsapp/router.go`)

Implementar um roteador que interpreta a mensagem de texto do usuário e decide
qual intent chamar. Manter SIMPLES — não precisa de NLP, use prefixos:

| Comando | Ação |
|---------|------|
| `--login <email> <senha>` | Autentica e vincula o telefone ao user_id |
| Texto livre sem prefixo | `core.Notes.Capture(ctx, userID, text)` — captura rápida |
| `--chat <texto>` | `core.Chat.Send(...)` — chat com IA, usando sink que acumula resposta |
| `--alerta <texto>` | `core.Alerts.Create(ctx, userID, text)` — alerta por linguagem natural |
| `--lembretes` | `core.Alerts.List(ctx, userID)` — lista alertas pendentes |
| `--notas` | `core.Notes.List(ctx, userID)` — lista últimas notas |
| `--busca <q>` | `core.Notes.Find(ctx, userID, q)` — busca notas |
| `--pergunta <q>` | `core.Notes.Ask(ctx, userID, q)` — busca + IA responde |
| `--ajuda` | Envia lista de comandos disponíveis |

### 3. Sink de chat para WhatsApp (`whatsappChatSink`)

O `core.Chat` espera um `core.ChatSink(func(ChatEvent) error)`. Para WhatsApp,
implemente um sink que:
- Acumula todos os `delta` num `strings.Builder`.
- Ignora `usage` e `note_proposed`/`alert_proposed` (ou emite um texto
  informando que uma nota/alerta foi proposta — simplifique).
- No `done`, envia o texto acumulado via `client.SendText`.
- No `error`, envia a mensagem de erro.

Não precisa de streaming via SSE — WhatsApp não suporta server push fácil.

### 4. Migration `0004_user_channels.sql`

```sql
CREATE TABLE IF NOT EXISTS user_channels (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel    TEXT NOT NULL,              -- 'whatsapp'
    address    TEXT NOT NULL,              -- telefone com DDI (ex.: 5511999999999)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(channel, address)
);
CREATE INDEX IF NOT EXISTS idx_user_channels_user ON user_channels(user_id);
```

### 5. Store methods (`internal/store/`)

Adicionar em `store/users.go` ou novo `store/channels.go`:

```go
func (s *Store) GetUserByChannel(ctx context.Context, channel, address string) (int64, error)
// SELECT user_id FROM user_channels WHERE channel=$1 AND address=$2

func (s *Store) LinkChannel(ctx context.Context, userID int64, channel, address string) error
// INSERT INTO user_channels (user_id, channel, address) VALUES ($1, $2, $3)
// ON CONFLICT (channel, address) DO UPDATE SET user_id = $1
```

### 6. Config

Adicionar ao `internal/config/config.go`:

```go
WhatsAppPhoneNumberID string // env WHATSAPP_PHONE_NUMBER_ID
WhatsAppToken         string // env WHATSAPP_TOKEN (system user access token)
WhatsAppVerifyToken   string // env WHATSAPP_VERIFY_TOKEN (seu próprio token p/ verificação do webhook)
```

### 7. Rotas HTTP

Em `cmd/server/main.go`, registrar as rotas do webhook **fora do middleware de
auth** (são públicas — o Meta precisa acessar sem token):

```go
// público (webhooks)
mux.HandleFunc("GET /webhook/whatsapp", whatsappHandler.HandleWebhook)
mux.HandleFunc("POST /webhook/whatsapp", whatsappHandler.HandleWebhook)
```

Dica: crie um segundo mux ou adicione as rotas públicas no mesmo mux do server,
fora do `protected`. Em `internal/httpapi/server.go`, exponha o `mux` ou um
método `RegisterPublic(pattern string, handler http.Handler)`.

## Setup para teste

1. Criar uma conta no **Meta for Developers** (https://developers.facebook.com)
2. Criar um app do tipo "Business"
3. Configurar WhatsApp Cloud API com um número de telefone de teste
4. Pegar `Phone Number ID` e `System User Access Token`
5. Usar ngrok ou similar para expor o servidor local ao webhook do Meta
6. Variáveis de ambiente:
   ```
   WHATSAPP_PHONE_NUMBER_ID=...
   WHATSAPP_TOKEN=...
   WHATSAPP_VERIFY_TOKEN=my_secure_token
   ```

## Entregáveis

- [ ] `internal/channels/whatsapp/client.go` — Client de envio
- [ ] `internal/channels/whatsapp/webhook.go` — Handler do webhook (GET + POST)
- [ ] `internal/channels/whatsapp/router.go` — Roteador de comandos
- [ ] `internal/channels/whatsapp/sink.go` — ChatSink para WhatsApp
- [ ] `migrations/0004_user_channels.sql`
- [ ] `internal/store/channels.go` — GetUserByChannel, LinkChannel
- [ ] Config atualizada (whatsapp fields em `config.go`)
- [ ] Rotas registradas em `cmd/server/main.go` ou `httpapi/server.go`
- [ ] `go build ./...` e `go vet ./...` passando

## Não fazer

- Não criar streaming SSE para WhatsApp (use o sink que acumula).
- Não colocar lógica de negócio no adapter — chame as intents do core.
- Não implementar áudio, imagem, documento, location — só texto.
- Não se preocupar com rate limiting do Meta API por ora.
