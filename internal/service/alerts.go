// Alerts implementa core.Alerts sobre o store. As mutações são CRUD puro
// (List/CreateProposed/Cancel/Done/Snooze); a criação a partir de linguagem
// natural (Create/CreateForNote) depende do parsing de tempo por IA e fica em
// 501 até a fase 2. O agendamento/disparo é do scheduler server-side (fase 3).
package service

import (
	"context"
	"time"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Alerts implementa core.Alerts.
type Alerts struct{ store *store.Store }

func NewAlerts(s *store.Store) *Alerts { return &Alerts{store: s} }

var _ core.Alerts = (*Alerts)(nil)

func (a *Alerts) List(ctx context.Context, userID int64) ([]core.Alert, error) {
	return a.store.ListAlerts(ctx, userID)
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

// --- dependem de IA (parsing de tempo em linguagem natural) — fase 2 --------

func (a *Alerts) Create(context.Context, int64, string) (core.CreateAlertResult, error) {
	return core.CreateAlertResult{}, core.ErrNotImplemented
}

func (a *Alerts) CreateForNote(context.Context, int64, int64, string) (core.CreateAlertResult, error) {
	return core.CreateAlertResult{}, core.ErrNotImplemented
}
