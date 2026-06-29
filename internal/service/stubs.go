package service

import (
	"context"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Alerts, Chat e History têm o esqueleto pronto (satisfazem o contrato) mas
// dependem de IA (parsing de tempo), scheduler+push e/ou tabelas ainda não
// portadas — entram nas fases 1.x/2. Por ora retornam core.ErrNotImplemented.

// Alerts implementa core.Alerts.
type Alerts struct{ store *store.Store }

func NewAlerts(s *store.Store) *Alerts { return &Alerts{store: s} }

var _ core.Alerts = (*Alerts)(nil)

func (a *Alerts) List(context.Context, int64) ([]core.Alert, error) {
	return nil, core.ErrNotImplemented
}
func (a *Alerts) Create(context.Context, int64, string) (core.CreateAlertResult, error) {
	return core.CreateAlertResult{}, core.ErrNotImplemented
}
func (a *Alerts) CreateForNote(context.Context, int64, int64, string) (core.CreateAlertResult, error) {
	return core.CreateAlertResult{}, core.ErrNotImplemented
}
func (a *Alerts) CreateProposed(context.Context, int64, string, string, string, *int64) (core.CreateAlertResult, error) {
	return core.CreateAlertResult{}, core.ErrNotImplemented
}
func (a *Alerts) Cancel(context.Context, int64, int64) error { return core.ErrNotImplemented }
func (a *Alerts) Done(context.Context, int64, int64) error   { return core.ErrNotImplemented }
func (a *Alerts) Snooze(context.Context, int64, int64, int) error {
	return core.ErrNotImplemented
}

// Chat implementa core.Chat.
type Chat struct{ store *store.Store }

func NewChat(s *store.Store) *Chat { return &Chat{store: s} }

var _ core.Chat = (*Chat)(nil)

func (c *Chat) Send(context.Context, int64, core.ChatInput, core.ChatSink) error {
	return core.ErrNotImplemented
}

// History implementa core.History.
type History struct{ store *store.Store }

func NewHistory(s *store.Store) *History { return &History{store: s} }

var _ core.History = (*History)(nil)

func (h *History) List(context.Context, int64) ([]core.Conversation, error) {
	return nil, core.ErrNotImplemented
}
func (h *History) Messages(context.Context, int64, int64) ([]core.Message, error) {
	return nil, core.ErrNotImplemented
}
func (h *History) Delete(context.Context, int64, int64) error { return core.ErrNotImplemented }
