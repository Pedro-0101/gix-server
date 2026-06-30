package httpapi

import "net/http"

type endpointDoc struct {
	Method      string        `json:"method"`
	Path        string        `json:"path"`
	Description string        `json:"description"`
	Auth        string        `json:"auth"`
	Body        *fieldDoc     `json:"body,omitempty"`
	Query       []paramDoc    `json:"query,omitempty"`
	Returns     string        `json:"returns"`
	Streaming   bool          `json:"streaming,omitempty"`
	Notes       string        `json:"notes,omitempty"`
}

type fieldDoc struct {
	Description string       `json:"description"`
	Fields      []paramDoc   `json:"fields,omitempty"`
}

type paramDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

var apiDocs = []endpointDoc{
	// --- público ---
	{
		Method: "POST", Path: "/v1/auth/signup",
		Description: "Cria uma conta e retorna tokens de acesso.",
		Auth: "none",
		Body: &fieldDoc{Description: "Credenciais de cadastro", Fields: []paramDoc{
			{Name: "email", Type: "string", Required: true, Description: "Email do usuário"},
			{Name: "password", Type: "string", Required: true, Description: "Senha (mínimo 8 caracteres)"},
		}},
		Returns: "201: { accessToken, refreshToken } | 400 | 409 (email duplicado)",
	},
	{
		Method: "POST", Path: "/v1/auth/login",
		Description: "Autentica e retorna tokens de acesso.",
		Auth: "none",
		Body: &fieldDoc{Description: "Credenciais de login", Fields: []paramDoc{
			{Name: "email", Type: "string", Required: true},
			{Name: "password", Type: "string", Required: true},
		}},
		Returns: "200: { accessToken, refreshToken } | 401 (credenciais inválidas)",
	},
	{
		Method: "POST", Path: "/v1/auth/refresh",
		Description: "Troca um refresh token por um novo par de tokens (rotação).",
		Auth: "none",
		Body: &fieldDoc{Description: "Token de refresh", Fields: []paramDoc{
			{Name: "refreshToken", Type: "string", Required: true},
		}},
		Returns: "200: { accessToken, refreshToken } | 401 (token inválido/expirado)",
	},
	// --- protegidas: health e docs ---
	{
		Method: "GET", Path: "/healthz",
		Description: "Health check. Retorna 'ok'.",
		Auth: "none", Returns: "200: 'ok'",
	},
	// --- notas ---
	{
		Method: "GET", Path: "/v1/notes",
		Description: "Lista notas do usuário com paginação.",
		Auth: "Bearer",
		Query: []paramDoc{
			{Name: "limit", Type: "int", Description: "Máximo de notas (1-200, default 50)"},
			{Name: "offset", Type: "int", Description: "Deslocamento (default 0)"},
		},
		Returns: "200: [{ id, title, content, tags, charLimit, createdAt, updatedAt }]",
	},
	{
		Method: "POST", Path: "/v1/notes",
		Description: "Cria uma nota manualmente (sem IA).",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Dados da nota", Fields: []paramDoc{
			{Name: "title", Type: "string", Required: true, Description: "Título da nota"},
			{Name: "content", Type: "string", Required: true, Description: "Conteúdo (Markdown)"},
			{Name: "tags", Type: "string[]", Description: "Tags (max 5, normalizadas lowercase)"},
		}},
		Returns: "201: CaptureResult { status, noteId, noteTitle, content, tags, usage }",
	},
	{
		Method: "GET", Path: "/v1/notes/graph",
		Description: "Grafo de notas ligadas por tags compartilhadas (visualização estilo Obsidian).",
		Auth: "Bearer",
		Returns: "200: { nodes: [{ id, title, tags }], edges: [{ source, target }] }",
	},
	{
		Method: "POST", Path: "/v1/notes/backfill-embeddings",
		Description: "Regera embeddings para todas as notas do usuário que ainda não têm vetor.",
		Auth: "Bearer",
		Returns: "200: { embedded: int }",
	},
	{
		Method: "GET", Path: "/v1/notes/find",
		Description: "Busca híbrida (FTS + similaridade vetorial fundidas por RRF).",
		Auth: "Bearer",
		Query: []paramDoc{
			{Name: "q", Type: "string", Required: true, Description: "Termo de busca"},
		},
		Returns: "200: [{ noteId, title, snippet, content, tags, score }]",
	},
	{
		Method: "GET", Path: "/v1/notes/ask",
		Description: "Pergunta respondida pela IA com base nas notas mais relevantes do usuário. Requer chave OpenRouter configurada (user_prefs.openrouter_key ou OPENROUTER_API_KEY no servidor).",
		Auth: "Bearer",
		Query: []paramDoc{
			{Name: "q", Type: "string", Required: true, Description: "Pergunta em linguagem natural"},
		},
		Returns: "200: AskResult { status (ok|no_api_key|empty|error), summary, sources, usage }",
	},
	{
		Method: "POST", Path: "/v1/notes/capture",
		Description: "Captura texto livre e usa IA para estruturar em nota (título + conteúdo + tags). Pode propor anexar a nota existente ou sinalizar overflow. Requer chave OpenRouter.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Texto livre", Fields: []paramDoc{
			{Name: "text", Type: "string", Required: true},
		}},
		Returns: "200: CaptureResult { status (created|attach_proposed|overflow_proposed|no_api_key|error), noteId, noteTitle, content, tags, attach, overflow, alert, usage }",
	},
	{
		Method: "GET", Path: "/v1/notes/{id}",
		Description: "Obtém uma nota pelo ID.",
		Auth: "Bearer",
		Returns: "200: Note | 404",
	},
	{
		Method: "PUT", Path: "/v1/notes/{id}",
		Description: "Atualiza título, conteúdo e/ou tags de uma nota.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Campos a atualizar", Fields: []paramDoc{
			{Name: "title", Type: "string", Description: "Novo título"},
			{Name: "content", Type: "string", Description: "Novo conteúdo (Markdown)"},
			{Name: "tags", Type: "string[]", Description: "Novas tags"},
		}},
		Returns: "200: Note | 404",
	},
	{
		Method: "DELETE", Path: "/v1/notes/{id}",
		Description: "Remove uma nota permanentemente.",
		Auth: "Bearer",
		Returns: "204 | 404",
	},
	{
		Method: "PUT", Path: "/v1/notes/{id}/char-limit",
		Description: "Define o limite de caracteres individual da nota (0 = usa o limite global do usuário).",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Limite", Fields: []paramDoc{
			{Name: "limit", Type: "int", Required: true, Description: "Limite de caracteres (0 = sem limite individual)"},
		}},
		Returns: "204 | 404",
	},
	{
		Method: "POST", Path: "/v1/notes/{id}/summarize",
		Description: "Pede à IA um resumo da nota em Markdown. Não altera a nota — o resultado é retornado para o cliente aplicar/desfazer. Requer chave OpenRouter.",
		Auth: "Bearer",
		Returns: "200: SummarizeResult { status (ok|no_api_key|empty|error), summary, usage }",
	},
	{
		Method: "POST", Path: "/v1/notes/{id}/tidy",
		Description: "Pede à IA para reorganizar/formatar a nota preservando TODA a informação. Não altera — resultado para aplicar/desfazer. Requer chave OpenRouter.",
		Auth: "Bearer",
		Returns: "200: TidyResult { status (ok|no_api_key|empty|error), content, usage }",
	},
	{
		Method: "POST", Path: "/v1/notes/{id}/append",
		Description: "Anexa conteúdo a uma nota existente, unindo tags. Se estourar o limite de caracteres, retorna overflow_proposed.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Conteúdo a anexar", Fields: []paramDoc{
			{Name: "content", Type: "string", Required: true},
			{Name: "tags", Type: "string[]", Description: "Tags adicionais"},
		}},
		Returns: "200: CaptureResult",
	},
	{
		Method: "POST", Path: "/v1/notes/{id}/overflow",
		Description: "Resolve overflow de uma nota: mode=split divide em notas temáticas, mode=summarize resume, mode=part2 cria parte 2. Requer chave OpenRouter para split e summarize.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Resolução de overflow", Fields: []paramDoc{
			{Name: "content", Type: "string", Required: true, Description: "Conteúdo que causou overflow"},
			{Name: "tags", Type: "string[]", Description: "Tags adicionais"},
			{Name: "mode", Type: "string", Required: true, Description: "split | summarize | part2"},
		}},
		Returns: "200: CaptureResult",
	},
	// --- alertas ---
	{
		Method: "GET", Path: "/v1/alerts",
		Description: "Lista lembretes do usuário com paginação.",
		Auth: "Bearer",
		Query: []paramDoc{
			{Name: "limit", Type: "int", Description: "Máximo (default 50)"},
			{Name: "offset", Type: "int", Description: "Deslocamento (default 0)"},
		},
		Returns: "200: [{ id, message, noteId, fireAt, recurrence, status, createdAt }]",
	},
	{
		Method: "POST", Path: "/v1/alerts",
		Description: "Cria um lembrete já estruturado (proposta confirmada). Sem IA.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Dados do lembrete", Fields: []paramDoc{
			{Name: "message", Type: "string", Required: true, Description: "Mensagem do lembrete"},
			{Name: "fireAt", Type: "string (ISO 8601)", Required: true, Description: "Data/hora de disparo com offset"},
			{Name: "recurrence", Type: "string", Description: "Regra de recorrência em JSON (ex: {\"freq\":\"daily\"})"},
			{Name: "noteId", Type: "int64?", Description: "ID da nota vinculada"},
		}},
		Returns: "201: CreateAlertResult { status, alertId, fireAtLocal, recurrence }",
	},
	{
		Method: "POST", Path: "/v1/alerts/parse",
		Description: "Cria um lembrete a partir de linguagem natural (ex: 'me lembre de comprar pão amanhã às 10h'). A IA extrai data/hora e recorrência. Requer chave OpenRouter.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Texto em linguagem natural", Fields: []paramDoc{
			{Name: "text", Type: "string", Required: true},
		}},
		Returns: "200: CreateAlertResult { status (created|no_api_key|unparseable|past|error), alertId, fireAtLocal, recurrence, usage }",
	},
	{
		Method: "POST", Path: "/v1/alerts/{id}/done",
		Description: "Marca lembrete como concluído.",
		Auth: "Bearer",
		Returns: "204 | 404",
	},
	{
		Method: "POST", Path: "/v1/alerts/{id}/cancel",
		Description: "Cancela um lembrete.",
		Auth: "Bearer",
		Returns: "204 | 404",
	},
	{
		Method: "POST", Path: "/v1/alerts/{id}/snooze",
		Description: "Adia um lembrete por N minutos.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Minutos de adiamento", Fields: []paramDoc{
			{Name: "minutes", Type: "int", Required: true, Description: "Minutos para adiar (> 0)"},
		}},
		Returns: "204 | 404 | 400",
	},
	{
		Method: "POST", Path: "/v1/notes/{id}/alert",
		Description: "Cria um lembrete vinculado a uma nota, com texto de quando (ex: 'daqui 1 hora'). Requer chave OpenRouter.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Texto de quando", Fields: []paramDoc{
			{Name: "text", Type: "string", Required: true, Description: "Expressão de tempo (ex: 'amanhã às 9h')"},
		}},
		Returns: "200: CreateAlertResult",
	},
	// --- conversas ---
	{
		Method: "GET", Path: "/v1/conversations",
		Description: "Lista conversas do usuário.",
		Auth: "Bearer",
		Query: []paramDoc{
			{Name: "limit", Type: "int", Description: "Máximo (default 50)"},
			{Name: "offset", Type: "int", Description: "Deslocamento (default 0)"},
		},
		Returns: "200: [{ id, title, model, createdAt }]",
	},
	{
		Method: "GET", Path: "/v1/conversations/{id}/messages",
		Description: "Mensagens de uma conversa.",
		Auth: "Bearer",
		Returns: "200: [{ id, role, content }] | 404",
	},
	{
		Method: "DELETE", Path: "/v1/conversations/{id}",
		Description: "Remove uma conversa e suas mensagens.",
		Auth: "Bearer",
		Returns: "204 | 404",
	},
	// --- modelos ---
	{
		Method: "GET", Path: "/v1/models",
		Description: "Lista modelos de IA disponíveis com preços.",
		Auth: "Bearer",
		Returns: "200: [{ id, inputPrice, outputPrice }]",
	},
	// --- preferências ---
	{
		Method: "GET", Path: "/v1/prefs",
		Description: "Obtém preferências do usuário.",
		Auth: "Bearer",
		Returns: "200: { model, language, systemPrompt, charLimit, chatMaxTokens, timezone, openrouterKey }",
	},
	{
		Method: "PUT", Path: "/v1/prefs",
		Description: "Atualiza preferências do usuário (merge parcial — campos omitidos mantêm o valor atual).",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Preferências (todos os campos opcionais)", Fields: []paramDoc{
			{Name: "model", Type: "string", Description: "Modelo de IA preferido (ex: 'google/gemini-2.5-flash-lite')"},
			{Name: "language", Type: "string", Description: "Idioma para prompts (ex: 'Português do Brasil', 'English')"},
			{Name: "systemPrompt", Type: "string", Description: "System prompt customizado (prependido ao prompt base)"},
			{Name: "charLimit", Type: "int", Description: "Limite global de caracteres por nota (default 8000)"},
			{Name: "chatMaxTokens", Type: "int", Description: "Limite de tokens de contexto por conversa (0 = usa default do servidor, 96000). O servidor trunca o histórico automaticamente ao atingir este limite."},
			{Name: "timezone", Type: "string", Description: "Fuso horário IANA (ex: 'America/Sao_Paulo')"},
			{Name: "openrouterKey", Type: "string", Description: "Chave de API OpenRouter do usuário (sk-or-v1-...). Se não definida, usa a chave do servidor (.env)."},
		}},
		Returns: "200: UserPrefs atualizado",
	},
	// --- chat (SSE) ---
	{
		Method: "POST", Path: "/v1/chat",
		Description: "Envia mensagem para a IA com streaming SSE. Requer chave OpenRouter. Eventos SSE: delta (texto incremental), done (conteúdo final), usage (custo), note_proposed (proposta de nota), alert_proposed (proposta de lembrete), error.",
		Auth: "Bearer",
		Body: &fieldDoc{Description: "Mensagem de chat", Fields: []paramDoc{
			{Name: "conversationId", Type: "int64", Description: "0 = nova conversa; >0 = continua conversa existente"},
			{Name: "text", Type: "string", Required: true, Description: "Mensagem do usuário"},
		}},
		Streaming: true,
		Returns: "text/event-stream (SSE)",
		Notes: "Rate limit: 2 req/s, burst 5. Conexão mantida até o fim do stream.",
	},
	// --- push (SSE) ---
	{
		Method: "GET", Path: "/v1/push",
		Description: "Stream SSE de eventos server-side (alertas disparados pelo scheduler). Conexão de longa duração.",
		Auth: "Bearer",
		Streaming: true,
		Returns: "text/event-stream (SSE)",
		Notes: "Rate limit: 2 req/s, burst 5. Mantenha uma conexão aberta para receber notificações push.",
	},
}

func (s *Server) getDocs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":   "gix-server",
		"auth":      "Bearer token via header Authorization: Bearer <accessToken>. Obtido via POST /v1/auth/login ou /v1/auth/signup. Renovado via POST /v1/auth/refresh.",
		"rateLimit": "Rotas normais: 10 req/s burst 20. Streaming (chat, push): 2 req/s burst 5.",
		"errors":    "400 (json inválido), 401 (não autenticado), 404 (recurso não encontrado), 409 (conflito), 429 (rate limit), 500 (erro interno)",
		"endpoints": apiDocs,
	})
}
