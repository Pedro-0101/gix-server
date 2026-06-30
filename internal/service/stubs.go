package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Pedro-0101/gix-server/internal/ai"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Chat implementa core.Chat com streaming SSE + tool-calls (create_note,
// create_alert).
type Chat struct {
	store *store.Store
	ai    AI
}

func NewChat(s *store.Store, aiDeps AI) *Chat { return &Chat{store: s, ai: aiDeps} }

var _ core.Chat = (*Chat)(nil)

// baseSystemPrompt define a identidade do assistente e as regras de uso das
// ferramentas durante o chat.
func baseSystemPrompt() string {
	return `Você é um assistente pessoal rápido e versátil.
Seu propósito PRINCIPAL é responder perguntas, fazer pesquisas rápidas, ajudar com dúvidas e fornecer informações.

Você tem duas ferramentas auxiliares:

[create_note] — USE com frequência. Sugira anotações sempre que:
- O usuário compartilhar qualquer ideia, aprendizado, decisão, plano ou insight
- O usuário der informações que possam ser úteis depois (links, comandos, dicas, receitas, listas, recomendações)
- O usuário mencionar algo que pretende fazer no futuro
- Durante a conversa surgir qualquer informação que o usuário possa querer consultar depois
- O usuário pedir explicitamente "anota isso", "registra", "guarda aí", "salva"
Apenas EVITE criar nota quando for pergunta trivial ou bate-papo casual sem informação relevante.
Na dúvida, sugira a nota — é melhor oferecer e o usuário recusar do que perder informação importante.

[create_alert] — USE quando:
- O usuário pedir "lembrete", "alarme", "alerta", "me lembre", "despertador"
- O usuário mencionar algo para fazer em um horário ou data específica
- O usuário usar expressões como "amanhã", "daqui a X horas", "às X horas"
NÃO use create_alert se não houver menção a tempo/horário.

REGRAS IMPORTANTES:
- Responda perguntas primeiro — as ferramentas são complementares
- Antes de chamar create_note, SEMPRE escreva na resposta algo como "Irei criar uma nota sobre [título] para registrar [motivo]..." e só depois chame a ferramenta
- Antes de chamar create_alert, SEMPRE escreva na resposta algo como "Irei criar um lembrete/alarme para [mensagem] em [data/hora]..." e só depois chame a ferramenta
- Não invente informações que não conhece
- Se não souber a resposta, diga que não sabe`
}

// chatTimeSystem injeta o horário local atual para o modelo resolver datas
// relativas ao chamar create_alert.
func chatTimeSystem(now time.Time, language string) ai.Message {
	stamp, zoneName, offsetH := localTimeHeader(now)
	return ai.Message{
		Role: "system",
		Content: fmt.Sprintf(
			"Data e hora locais atuais: %s. Fuso: %s (UTC%+d). Idioma: %s.",
			stamp, zoneName, offsetH, language),
	}
}

// chatTools devolve os schemas das ferramentas disponíveis no chat.
func chatTools() []ai.Tool {
	return []ai.Tool{
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "create_note",
				Description: "Cria uma anotação quando o usuário quer registrar uma informação importante (ideia, aprendizado, decisão etc.). Extraia um título curto, o conteúdo em Markdown e de 1 a 5 tags temáticas.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Título curto da anotação"},"content":{"type":"string","description":"Conteúdo em Markdown"},"tags":{"type":"array","items":{"type":"string"},"description":"1 a 5 tags temáticas, minúsculas, sem #"}},"required":["title","content","tags"]}`),
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "create_alert",
				Description: "Agenda um lembrete/alarme quando o usuário pede para ser lembrado de algo num horário ou data. Resolva datas relativas a partir do horário local atual informado no system prompt.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"},"fire_at":{"type":"string","description":"ISO 8601 com offset"},"recurrence":{"type":["object","null"]}},"required":["message","fire_at"]}`),
			},
		},
	}
}

// Send faz uma chamada streaming para a IA, emite eventos via sink, persiste
// a conversa e as mensagens no banco. Tool-calls geram eventos note_proposed /
// alert_proposed sem persistir — a confirmação vem por endpoints separados.
func (c *Chat) Send(ctx context.Context, userID int64, in core.ChatInput, sink core.ChatSink) error {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil
	}
	key := c.ai.resolveKey(ctx, c.store, userID)
	if key == "" {
		return sink(core.ChatEvent{Type: "error", Err: "chave de IA não configurada"})
	}

	// Carrega preferências do usuário
	prefs, err := c.store.GetUserPrefs(ctx, userID)
	if err != nil {
		prefs = store.UserPrefs{}
	}
	language := prefs.Language
	model := prefs.Model
	if model == "" {
		model = c.ai.Model
	}

	// Carrega histórico se conversationID > 0
	var convID int64
	history := []ai.Message{}
	if in.ConversationID != 0 {
		convID = in.ConversationID
		msgs, err := c.store.GetMessages(ctx, userID, convID)
		if err == nil {
			for _, m := range msgs {
				role := m.Role
				if role != "user" && role != "assistant" {
					role = "user"
				}
				history = append(history, ai.Message{Role: role, Content: m.Content})
			}
		}
	}

	// Cria conversa se nova
	if convID == 0 {
		title := extractTitle(text)
		id, err := c.store.CreateConversation(ctx, userID, title, model)
		if err == nil {
			convID = id
		}
	}

	// Persiste mensagem do usuário
	if convID != 0 {
		_ = c.store.AddMessage(ctx, convID, "user", text)
	}

	// Monta mensagens para a IA: o prompt customizado do usuário (se houver)
	// é prepended ao baseSystemPrompt para preservar as definições das
	// ferramentas (create_note, create_alert).
	systemContent := baseSystemPrompt()
	if prefs.SystemPrompt != "" {
		systemContent = prefs.SystemPrompt + "\n\n" + systemContent
	}
	// Trunca o histórico se exceder o limite de tokens de contexto.
	maxTokens := resolveMaxTokens(prefs.ChatMaxTokens)
	reserved := estimateTokens(systemContent) + estimateTokens(text) + 100
	budget := maxTokens - reserved
	if budget < 0 {
		budget = 0
	}
	history = trimHistory(history, budget)
	msgs := []ai.Message{
		{Role: "system", Content: systemContent},
		chatTimeSystem(time.Now(), lang(language)),
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, ai.Message{Role: "user", Content: text})

	// Streaming
	var sb strings.Builder
	usagePtr, toolCalls, streamErr := c.ai.Client.StreamTools(ctx, key, model, msgs, chatTools(), func(delta string) {
		sb.WriteString(delta)
		sink(core.ChatEvent{Type: "delta", Delta: delta}) //nolint:errcheck
	})

	full := sb.String()

	// Usage
	usage := c.ai.usage(usagePtr)
	sink(core.ChatEvent{Type: "usage", Usage: &usage}) //nolint:errcheck

	if streamErr != nil {
		if ctx.Err() == context.Canceled {
			if full != "" && convID != 0 {
				_ = c.store.AddMessage(ctx, convID, "assistant", full)
			}
			return nil
		}
		sink(core.ChatEvent{Type: "error", Err: streamErr.Error()}) //nolint:errcheck
		return nil
	}

	// Tool-calls: cria eventos note_proposed / alert_proposed
	for _, tc := range toolCalls {
		switch tc.Name {
		case "create_note":
			var dec struct {
				Title   string   `json:"title"`
				Content string   `json:"content"`
				Tags    []string `json:"tags"`
			}
			if json.Unmarshal([]byte(tc.Arguments), &dec) == nil {
				title := strings.TrimSpace(dec.Title)
				content := strings.TrimSpace(dec.Content)
				if title != "" && content != "" {
					sink(core.ChatEvent{ //nolint:errcheck
						Type: "note_proposed",
						Note: &core.CaptureResult{
							NoteTitle: title,
							Content:   content,
							Tags:      normalizeTags(dec.Tags),
						},
					})
				}
			}
		case "create_alert":
			var alertDec struct {
				Message      string          `json:"message"`
				FireAt       string          `json:"fire_at"`
				FireAtCamel  string          `json:"fireAt"`
				Recurrence   json.RawMessage `json:"recurrence"`
			}
			if json.Unmarshal([]byte(tc.Arguments), &alertDec) == nil {
				fireAt := alertDec.FireAt
				if fireAt == "" {
					fireAt = alertDec.FireAtCamel
				}
				msg := strings.TrimSpace(alertDec.Message)
				fireAt = strings.TrimSpace(fireAt)
				rec := ""
				if len(alertDec.Recurrence) > 0 && string(alertDec.Recurrence) != "null" {
					rec = string(alertDec.Recurrence)
				}
				if msg != "" && fireAt != "" {
					sink(core.ChatEvent{ //nolint:errcheck
						Type: "alert_proposed",
						Alert: &core.AlertProposal{
							Message:    msg,
							FireAt:     fireAt,
							Recurrence: rec,
						},
					})
				}
			}
		}
	}

	// Emite done com o texto final
	if full == "" && len(toolCalls) > 0 {
		full = "(ações executadas)"
	}
	sink(core.ChatEvent{Type: "done", Content: full}) //nolint:errcheck

	// Persiste resposta do assistente
	if full != "" && convID != 0 {
		_ = c.store.AddMessage(ctx, convID, "assistant", full)
	}

	return nil
}
