// Package service implementa as intents do core (internal/core) sobre o store
// Postgres. Fica separado de core pra evitar ciclo de import: store depende de
// core (tipos), e service depende de core + store.
//
// Fase 1: CRUD de notas e grafo funcionam. As intents que dependem de IA/
// embeddings (Capture, Find, Ask, Summarize, Tidy, ResolveOverflow) retornam
// core.ErrNotImplemented até a fase 2 (relay de IA + busca server-side).
package service

import (
	"context"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Notes implementa core.Notes.
type Notes struct {
	store *store.Store
}

func NewNotes(s *store.Store) *Notes { return &Notes{store: s} }

var _ core.Notes = (*Notes)(nil)

// --- CRUD (sem IA) ---------------------------------------------------------

func (n *Notes) List(ctx context.Context, userID int64) ([]core.Note, error) {
	return n.store.ListNotes(ctx, userID)
}

func (n *Notes) Get(ctx context.Context, userID, id int64) (core.Note, error) {
	return n.store.GetNote(ctx, userID, id)
}

func (n *Notes) Update(ctx context.Context, userID, id int64, title, content string, tags []string) (core.Note, error) {
	return n.store.UpdateNote(ctx, userID, id, title, content, tags)
}

func (n *Notes) Delete(ctx context.Context, userID, id int64) error {
	return n.store.DeleteNote(ctx, userID, id)
}

func (n *Notes) SetCharLimit(ctx context.Context, userID, id int64, limit int) error {
	return n.store.SetCharLimit(ctx, userID, id, limit)
}

// CreateFromProposal grava uma nota a partir de campos já prontos (criação
// manual ou confirmação de uma proposta de captura) — sem chamar IA.
func (n *Notes) CreateFromProposal(ctx context.Context, userID int64, title, content string, tags []string) (core.CaptureResult, error) {
	note, err := n.store.CreateNote(ctx, userID, title, content, tags, 0)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	return core.CaptureResult{
		Status:    "created",
		NoteID:    note.ID,
		NoteTitle: note.Title,
		Content:   note.Content,
		Tags:      note.Tags,
	}, nil
}

// AppendTo anexa conteúdo a uma nota existente e faz união das tags (sem IA;
// re-embedding entra na fase 2). Não aplica overflow — isso depende de IA.
func (n *Notes) AppendTo(ctx context.Context, userID, targetID int64, content string, tags []string) (core.CaptureResult, error) {
	note, err := n.store.GetNote(ctx, userID, targetID)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	merged := note.Content
	if content != "" {
		merged += "\n\n" + content
	}
	updated, err := n.store.UpdateNote(ctx, userID, targetID, note.Title, merged, unionTags(note.Tags, tags))
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	return core.CaptureResult{
		Status:    "created",
		NoteID:    updated.ID,
		NoteTitle: updated.Title,
		Content:   updated.Content,
		Tags:      updated.Tags,
	}, nil
}

// Graph monta nós + arestas: duas notas se ligam se compartilham >=1 tag.
func (n *Notes) Graph(ctx context.Context, userID int64) (core.GraphData, error) {
	notes, err := n.store.GraphNotes(ctx, userID)
	if err != nil {
		return core.GraphData{}, err
	}
	g := core.GraphData{Nodes: []core.GraphNode{}, Edges: []core.GraphEdge{}} // nunca nil
	for _, nt := range notes {
		g.Nodes = append(g.Nodes, core.GraphNode{ID: nt.ID, Title: nt.Title, Tags: nt.Tags})
	}
	for i := 0; i < len(notes); i++ {
		for j := i + 1; j < len(notes); j++ {
			if shareTag(notes[i].Tags, notes[j].Tags) {
				g.Edges = append(g.Edges, core.GraphEdge{Source: notes[i].ID, Target: notes[j].ID})
			}
		}
	}
	return g, nil
}

// --- dependem de IA/embeddings (fase 2) ------------------------------------

func (n *Notes) Capture(context.Context, int64, string) (core.CaptureResult, error) {
	return core.CaptureResult{}, core.ErrNotImplemented
}

func (n *Notes) Find(context.Context, int64, string) ([]core.SearchResult, error) {
	return nil, core.ErrNotImplemented
}

func (n *Notes) Ask(context.Context, int64, string) (core.AskResult, error) {
	return core.AskResult{}, core.ErrNotImplemented
}

func (n *Notes) Summarize(context.Context, int64, int64) (core.SummarizeResult, error) {
	return core.SummarizeResult{}, core.ErrNotImplemented
}

func (n *Notes) Tidy(context.Context, int64, int64) (core.TidyResult, error) {
	return core.TidyResult{}, core.ErrNotImplemented
}

func (n *Notes) ResolveOverflow(context.Context, int64, int64, string, []string, string) (core.CaptureResult, error) {
	return core.CaptureResult{}, core.ErrNotImplemented
}

// --- helpers ---------------------------------------------------------------

func unionTags(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range append(append([]string{}, a...), b...) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func shareTag(a, b []string) bool {
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	for _, t := range b {
		if set[t] {
			return true
		}
	}
	return false
}
