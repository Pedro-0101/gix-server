package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Pedro-0101/gix-server/internal/auth"
)

type noteInput struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type charLimitInput struct {
	Limit int `json:"limit"`
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
	notes, err := s.core.Notes.List(r.Context(), userID)
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
