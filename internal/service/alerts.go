// Alerts implementa core.Alerts sobre o store. As mutações são CRUD puro
// (List/CreateProposed/Cancel/Done/Snooze); a criação a partir de linguagem
// natural (Create/CreateForNote) depende do parsing de tempo por IA e fica em
// 501 até a fase 2. O agendamento/disparo é do scheduler server-side (fase 3).
package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

const gracePeriod = 15 * time.Second

// Alerts implementa core.Alerts.
type Alerts struct {
	store *store.Store
	ai    AI
}

func NewAlerts(s *store.Store, aiDeps AI) *Alerts { return &Alerts{store: s, ai: aiDeps} }

var _ core.Alerts = (*Alerts)(nil)

func (a *Alerts) loadLang(ctx context.Context, userID int64) string {
	prefs, err := a.store.GetUserPrefs(ctx, userID)
	if err != nil {
		return ""
	}
	return prefs.Language
}

func (a *Alerts) List(ctx context.Context, userID int64, opts core.ListOptions) ([]core.Alert, error) {
	return a.store.ListAlerts(ctx, userID, store.Pagination{Limit: opts.Limit, Offset: opts.Offset})
}

// CreateProposed grava um lembrete já estruturado (vindo de uma proposta de
// captura/chat já confirmada): mensagem + fire_at ISO + recorrência. Sem IA.
func (a *Alerts) CreateProposed(ctx context.Context, userID int64, message, fireAtISO, recurrence string, noteID *int64) (core.CreateAlertResult, error) {
	fireAt, err := time.Parse(time.RFC3339, fireAtISO)
	if err != nil {
		return core.CreateAlertResult{Status: "unparseable", Message: "fireAt deve ser ISO 8601 (RFC3339)"}, nil
	}
	if fireAt.Before(time.Now()) {
		return core.CreateAlertResult{Status: "past", Message: "o horário do lembrete já passou"}, nil
	}
	created, err := a.store.CreateAlert(ctx, core.Alert{
		Message:    message,
		NoteID:     noteID,
		FireAt:     fireAt,
		Recurrence: recurrence,
	}, userID)
	if err != nil {
		return core.CreateAlertResult{Status: "error", Message: err.Error()}, err
	}
	return core.CreateAlertResult{
		Status:      "created",
		AlertID:     created.ID,
		FireAtLocal: created.FireAt.In(fireAt.Location()).Format(time.RFC3339),
		Recurrence:  created.Recurrence,
	}, nil
}

func (a *Alerts) Cancel(ctx context.Context, userID, id int64) error {
	return a.store.SetAlertStatus(ctx, userID, id, "cancelled")
}

func (a *Alerts) Done(ctx context.Context, userID, id int64) error {
	return a.store.SetAlertStatus(ctx, userID, id, "done")
}

// Snooze reagenda o alerta p/ daqui a `minutes` minutos, mantendo-o pendente.
func (a *Alerts) Snooze(ctx context.Context, userID, id int64, minutes int) error {
	return a.store.UpdateAlertFireAt(ctx, userID, id, time.Now().Add(time.Duration(minutes)*time.Minute))
}

// --- dependem de IA (parsing de tempo em linguagem natural) ----------------

// alertDecision é o JSON que a IA devolve.
type alertDecision struct {
	Message    string `json:"message"`
	FireAt     string `json:"fireAt"`
	Recurrence string `json:"recurrence"`
}

// UnmarshalJSON aceita fireAt e fire_at (alguns modelos usam snake_case).
func (d *alertDecision) UnmarshalJSON(b []byte) error {
	type raw alertDecision
	if err := json.Unmarshal(b, (*raw)(d)); err != nil {
		return err
	}
	// fallback: tentar ler de um mapa para aceitar fire_at
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil // raw já pode ter preenchido
	}
	if d.FireAt == "" {
		if rawAt, ok := m["fire_at"]; ok {
			json.Unmarshal(rawAt, &d.FireAt) //nolint:errcheck
		}
	}
	if d.Message == "" {
		if rawMsg, ok := m["message"]; ok {
			json.Unmarshal(rawMsg, &d.Message) //nolint:errcheck
		}
	}
	return nil
}

// parseWhen chama a IA para extrair data/hora de um texto em linguagem natural.
// Retorna o decision e o Usage; se a IA não conseguir parsear, fireAt vem vazio.
func (a *Alerts) parseWhen(ctx context.Context, userID int64, text, ctxMsg string) (alertDecision, core.Usage, error) {
	loc, err := a.userLocation(ctx, userID)
	if err != nil {
		return alertDecision{}, core.Usage{}, err
	}
	now := time.Now().In(loc)
	language := a.loadLang(ctx, userID)

	raw, usage, err := a.ai.complete(ctx, buildAlertPrompt(text, ctxMsg, now, lang(language)))
	if err != nil {
		return alertDecision{}, core.Usage{}, err
	}

	var d alertDecision
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return alertDecision{}, usage, nil // não achou data no texto → fireAt vazio
	}
	d.Message = strings.TrimSpace(d.Message)
	d.FireAt = strings.TrimSpace(d.FireAt)
	d.Recurrence = strings.TrimSpace(d.Recurrence)
	return d, usage, nil
}

// userLocation resolve o fuso do usuário; inválido/ausente cai para UTC.
func (a *Alerts) userLocation(ctx context.Context, userID int64) (*time.Location, error) {
	tz, err := a.store.UserTimezone(ctx, userID)
	if err != nil {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

// Create cria um lembrete a partir de linguagem natural (ex.: "me lembre de
// comprar pão amanhã às 10h"). Usa IA para extrair data/hora e recorrência.
func (a *Alerts) Create(ctx context.Context, userID int64, text string) (core.CreateAlertResult, error) {
	if !a.ai.hasKey() {
		return core.CreateAlertResult{Status: "no_api_key"}, nil
	}
	if strings.TrimSpace(text) == "" {
		return core.CreateAlertResult{Status: "unparseable", Message: "texto vazio"}, nil
	}

	dec, usage, err := a.parseWhen(ctx, userID, text, "")
	if err != nil {
		return core.CreateAlertResult{Status: "error", Message: err.Error()}, err
	}
	if dec.FireAt == "" {
		return core.CreateAlertResult{
			Status:  "unparseable",
			Message: "não foi possível extrair data/hora do texto",
			Usage:   usage,
		}, nil
	}

	return a.createFromDecision(ctx, userID, dec.Message, dec.FireAt, dec.Recurrence, nil, usage)
}

// CreateForNote cria um lembrete vinculado a uma nota, com texto de quando
// (ex.: "daqui 1 hora", "amanhã às 9h"). Usa o título da nota como mensagem
// default e o conteúdo como contexto.
func (a *Alerts) CreateForNote(ctx context.Context, userID, noteID int64, whenText string) (core.CreateAlertResult, error) {
	if !a.ai.hasKey() {
		return core.CreateAlertResult{Status: "no_api_key"}, nil
	}

	note, err := a.store.GetNote(ctx, userID, noteID)
	if err != nil {
		return core.CreateAlertResult{Status: "error", Message: err.Error()}, err
	}

	ctxMsg := note.Title
	if note.Content != "" {
		ctxMsg += "\n\n" + note.Content
	}

	dec, usage, err := a.parseWhen(ctx, userID, whenText, ctxMsg)
	if err != nil {
		return core.CreateAlertResult{Status: "error", Message: err.Error()}, err
	}
	if dec.FireAt == "" {
		return core.CreateAlertResult{
			Status:  "unparseable",
			Message: "não foi possível extrair data/hora do texto",
			Usage:   usage,
		}, nil
	}

	message := dec.Message
	if message == "" {
		message = note.Title
	}

	return a.createFromDecision(ctx, userID, message, dec.FireAt, dec.Recurrence, &noteID, usage)
}

// createFromDecision valida o decision e persiste o alerta.
func (a *Alerts) createFromDecision(ctx context.Context, userID int64, message, fireAtISO, recurrence string, noteID *int64, usage core.Usage) (core.CreateAlertResult, error) {
	loc, _ := a.userLocation(ctx, userID)

	fireAt, err := time.Parse(time.RFC3339, fireAtISO)
	if err != nil {
		fireAt, err = time.Parse("2006-01-02T15:04:05", fireAtISO)
		if err != nil {
			return core.CreateAlertResult{
				Status:  "unparseable",
				Message: "formato de data/hora inválido",
				Usage:   usage,
			}, nil
		}
	}

	now := time.Now().In(loc)
	recurring := recurrence != ""
	isPast := fireAt.Before(now.Add(-gracePeriod))

	if isPast && !recurring {
		return core.CreateAlertResult{
			Status:  "past",
			Message: "o horário do lembrete já passou",
			Usage:   usage,
		}, nil
	}

	created, err := a.store.CreateAlert(ctx, core.Alert{
		Message:    message,
		NoteID:     noteID,
		FireAt:     fireAt,
		Recurrence: recurrence,
	}, userID)
	if err != nil {
		return core.CreateAlertResult{Status: "error", Message: err.Error()}, err
	}

	return core.CreateAlertResult{
		Status:      "created",
		AlertID:     created.ID,
		FireAtLocal: created.FireAt.In(loc).Format(time.RFC3339),
		Recurrence:  created.Recurrence,
		Usage:       usage,
	}, nil
}
