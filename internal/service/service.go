package service

import (
	"github.com/Pedro-0101/gix-server/internal/ai"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/embed"
	"github.com/Pedro-0101/gix-server/internal/gcal"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// AI agrupa as dependências de IA compartilhadas pelas intents (relay + modelo).
// É injetada nos domínios que precisam de IA (Notes/Alerts/Chat); o client é só
// HTTP para a OpenRouter e o modelo vem do config.
type AI struct {
	Client   *ai.Client
	Model    string
	Embedder *embed.Embedder // nil se ONNX não estiver disponível
}

// ListOptions encapsula paginação para listagens.
type ListOptions = core.ListOptions

// ParseListOptions normaliza limit e offset: mínimo 1 e 0 respectivamente,
// máximo 200 para limit, default 50.
func ParseListOptions(limit, offset int) ListOptions {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return ListOptions{Limit: limit, Offset: offset}
}

// NewCore monta o core a partir do store e das deps de IA, ligando cada domínio
// à sua implementação. Notes/Alerts/Chat recebem o relay de IA; History é CRUD
// puro sobre o store.
func NewCore(s *store.Store, aiDeps AI, gcalClient *gcal.Client) *core.Core {
	return &core.Core{
		Notes:   NewNotes(s, aiDeps),
		Alerts:  NewAlerts(s, aiDeps, gcalClient),
		Chat:    NewChat(s, aiDeps),
		History: NewHistory(s),
	}
}
