package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/service"
)

type noteInput struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type charLimitInput struct {
	Limit int `json:"limit"`
}

type captureInput struct {
	Text string `json:"text"`
}

type appendInput struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type overflowInput struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Mode    string   `json:"mode"`
}

// userAndID extrai o usuário autenticado e o {id} da rota. Em falha, já responde.
func userAndID(w http.ResponseWriter, r *http.Request) (userID, id int64, ok bool) {
	userID, found := auth.UserID(r.Context())
	if !found {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return 0, 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return 0, 0, false
	}
	return userID, id, true
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	opts := service.ParseListOptions(limit, offset)
	notes, err := s.core.Notes.List(r.Context(), userID, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var in noteInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Notes.CreateFromProposal(r.Context(), userID, in.Title, in.Content, in.Tags)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	note, err := s.core.Notes.Get(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in noteInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	note, err := s.core.Notes.Update(r.Context(), userID, id, in.Title, in.Content, in.Tags)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	if err := s.core.Notes.Delete(r.Context(), userID, id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setCharLimit(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in charLimitInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	if err := s.core.Notes.SetCharLimit(r.Context(), userID, id, in.Limit); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	g, err := s.core.Notes.Graph(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) findNotes(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	query := r.URL.Query().Get("q")
	results, err := s.core.Notes.Find(r.Context(), userID, query)
	if err != nil {
		writeErr(w, err)
		return
	}
	if results == nil {
		results = []core.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) askNotes(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	query := r.URL.Query().Get("q")
	result, err := s.core.Notes.Ask(r.Context(), userID, query)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// captureNote é o POST /v1/notes/capture: recebe texto livre, IA estrutura em
// nota (ou propõe attach/overflow). Roteamento similar ao notes_route do gix.
func (s *Server) captureNote(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var in captureInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Notes.Capture(r.Context(), userID, in.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// backfillEmbeddings gera embeddings para notas sem vetor (admin). Requer
// autenticação. Retorna o total de notas embedadas.
func (s *Server) backfillEmbeddings(w http.ResponseWriter, r *http.Request) {
	notesSvc, ok := s.core.Notes.(*service.Notes)
	if !ok {
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	total, err := notesSvc.BackfillEmbeddings(r.Context(), 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"embedded": total})
}

func (s *Server) summarizeNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	res, err := s.core.Notes.Summarize(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) tidyNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	res, err := s.core.Notes.Tidy(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) appendToNote(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in appendInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Notes.AppendTo(r.Context(), userID, id, in.Content, in.Tags)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) resolveOverflow(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in overflowInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Notes.ResolveOverflow(r.Context(), userID, id, in.Content, in.Tags, in.Mode)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
