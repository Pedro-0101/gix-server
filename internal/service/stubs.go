package service

import (
	"context"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Chat depende do relay de IA + streaming (SSE) — entra na fase 2. Por ora
// retorna core.ErrNotImplemented. Alerts e History já saíram daqui (ver
// alerts.go/history.go): são CRUD puro sobre o store.

// Chat implementa core.Chat.
type Chat struct{ store *store.Store }

func NewChat(s *store.Store) *Chat { return &Chat{store: s} }

var _ core.Chat = (*Chat)(nil)

func (c *Chat) Send(context.Context, int64, core.ChatInput, core.ChatSink) error {
	return core.ErrNotImplemented
}
