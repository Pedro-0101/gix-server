package service

import (
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// NewCore monta o core a partir do store, ligando cada domínio à sua
// implementação. É aqui que, no futuro, as deps de IA/embeddings serão injetadas
// nos domínios que precisam (Notes/Alerts/Chat).
func NewCore(s *store.Store) *core.Core {
	return &core.Core{
		Notes:   NewNotes(s),
		Alerts:  NewAlerts(s),
		Chat:    NewChat(s),
		History: NewHistory(s),
	}
}
