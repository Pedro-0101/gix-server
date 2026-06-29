// History implementa core.History sobre o store: conversas passadas e suas
// mensagens. CRUD puro, escopado por usuário — a geração de mensagens (chat por
// IA) é da intent Chat (fase 2).
package service

import (
	"context"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// History implementa core.History.
type History struct{ store *store.Store }

func NewHistory(s *store.Store) *History { return &History{store: s} }

var _ core.History = (*History)(nil)

func (h *History) List(ctx context.Context, userID int64) ([]core.Conversation, error) {
	return h.store.ListConversations(ctx, userID)
}

func (h *History) Messages(ctx context.Context, userID, conversationID int64) ([]core.Message, error) {
	return h.store.GetMessages(ctx, userID, conversationID)
}

func (h *History) Delete(ctx context.Context, userID, conversationID int64) error {
	return h.store.DeleteConversation(ctx, userID, conversationID)
}
