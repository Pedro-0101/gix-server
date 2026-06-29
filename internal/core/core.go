package core

import "context"

// userID escopado: TODA intent recebe o usuário dono dos dados. Os adapters de
// canal resolvem a identidade (JWT no desktop/web, vínculo de conta nos bots)
// e repassam o userID; o core nunca confia no canal pra isolar dados — ele
// sempre filtra por userID nas queries.

// ChatEvent é um evento do streaming de chat. O core emite estes eventos de
// forma agnóstica; cada canal traduz: desktop/web reemitem via SSE -> eventos
// Wails (chat:delta/done/error), bots acumulam e mandam a resposta completa.
type ChatEvent struct {
	Type    string // delta | done | error | usage | note_proposed | alert_proposed
	Delta   string // texto incremental (Type=delta)
	Content string // conteúdo final (Type=done)
	Err     string // mensagem de erro (Type=error)
	Usage   *Usage // (Type=usage)
	Note    *CaptureResult // tool-call create_note vira proposta (Type=note_proposed)
	Alert   *AlertProposal // tool-call create_alert vira proposta (Type=alert_proposed)
}

// ChatSink recebe os eventos do chat. É o ponto de extensão de transporte:
// uma implementação escreve SSE, outra acumula pra um bot. Retornar erro
// aborta o stream.
type ChatSink func(ChatEvent) error

// ChatInput é a entrada de uma rodada de chat.
type ChatInput struct {
	ConversationID int64 // 0 = nova conversa
	Text           string
}

// Notes são as intents de notas — o mesmo conjunto que o NotesService do gix
// expõe hoje, menos a cola de UI/IPC.
type Notes interface {
	Capture(ctx context.Context, userID int64, text string) (CaptureResult, error)
	Find(ctx context.Context, userID int64, query string) ([]SearchResult, error)
	Ask(ctx context.Context, userID int64, query string) (AskResult, error)

	List(ctx context.Context, userID int64) ([]Note, error)
	Get(ctx context.Context, userID, id int64) (Note, error)
	Update(ctx context.Context, userID, id int64, title, content string, tags []string) (Note, error)
	Delete(ctx context.Context, userID, id int64) error
	SetCharLimit(ctx context.Context, userID, id int64, limit int) error

	Summarize(ctx context.Context, userID, id int64) (SummarizeResult, error)
	Tidy(ctx context.Context, userID, id int64) (TidyResult, error)

	// Operações sem IA usadas após uma proposta de captura/overflow.
	CreateFromProposal(ctx context.Context, userID int64, title, content string, tags []string) (CaptureResult, error)
	AppendTo(ctx context.Context, userID, targetID int64, content string, tags []string) (CaptureResult, error)
	ResolveOverflow(ctx context.Context, userID, targetID int64, content string, tags []string, mode string) (CaptureResult, error)

	Graph(ctx context.Context, userID int64) (GraphData, error)
}

// Alerts são as intents de lembretes. O agendamento/disparo roda no scheduler
// server-side; estas intents só criam/mutam os registros.
type Alerts interface {
	List(ctx context.Context, userID int64) ([]Alert, error)
	Create(ctx context.Context, userID int64, text string) (CreateAlertResult, error)
	CreateForNote(ctx context.Context, userID, noteID int64, whenText string) (CreateAlertResult, error)
	CreateProposed(ctx context.Context, userID int64, message, fireAtISO, recurrence string, noteID *int64) (CreateAlertResult, error)
	Cancel(ctx context.Context, userID, id int64) error
	Done(ctx context.Context, userID, id int64) error
	Snooze(ctx context.Context, userID, id int64, minutes int) error
}

// Chat é a intent de conversa. O streaming é entregue via sink, transporte-agnóstico.
type Chat interface {
	Send(ctx context.Context, userID int64, in ChatInput, sink ChatSink) error
}

// History são as conversas passadas.
type History interface {
	List(ctx context.Context, userID int64) ([]Conversation, error)
	Messages(ctx context.Context, userID, conversationID int64) ([]Message, error)
	Delete(ctx context.Context, userID, conversationID int64) error
}

// Core agrega todas as intents. Um adapter de canal recebe um *Core e expõe o
// que aquele canal suporta, chamando c.Notes.Find(...), c.Alerts.List(...), etc.
// É struct (não interface embutida) porque Notes/Alerts/History compartilham o
// nome List com assinaturas diferentes — agrupar por campo evita a colisão e
// deixa explícito de qual domínio é cada intent.
type Core struct {
	Notes   Notes
	Alerts  Alerts
	Chat    Chat
	History History
}
