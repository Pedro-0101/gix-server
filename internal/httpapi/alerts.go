package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/service"
)

// alertInput é a criação de um lembrete já estruturado (proposta confirmada).
type alertInput struct {
	Message    string `json:"message"`
	FireAt     string `json:"fireAt"` // ISO 8601 (RFC3339) com offset
	Recurrence string `json:"recurrence"`
	NoteID     *int64 `json:"noteId"`
}

type alertParseInput struct {
	Text string `json:"text"`
}

type snoozeInput struct {
	Minutes int `json:"minutes"`
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	opts := service.ParseListOptions(limit, offset)
	alerts, err := s.core.Alerts.List(r.Context(), userID, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (s *Server) createAlert(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var in alertInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Alerts.CreateProposed(r.Context(), userID, in.Message, in.FireAt, in.Recurrence, in.NoteID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) doneAlert(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	if err := s.core.Alerts.Done(r.Context(), userID, id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelAlert(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	if err := s.core.Alerts.Cancel(r.Context(), userID, id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) snoozeAlert(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in snoozeInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	if in.Minutes <= 0 {
		http.Error(w, "minutes deve ser > 0", http.StatusBadRequest)
		return
	}
	if err := s.core.Alerts.Snooze(r.Context(), userID, id, in.Minutes); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseAlert cria um lembrete a partir de linguagem natural.
func (s *Server) parseAlert(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var in alertParseInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Alerts.Create(r.Context(), userID, in.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// createNoteAlert cria um lembrete vinculado a uma nota, com texto de quando.
func (s *Server) createNoteAlert(w http.ResponseWriter, r *http.Request) {
	userID, noteID, ok := userAndID(w, r)
	if !ok {
		return
	}
	var in alertParseInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	res, err := s.core.Alerts.CreateForNote(r.Context(), userID, noteID, in.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
