package httpapi

import (
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/auth"
)

// alertInput é a criação de um lembrete já estruturado (proposta confirmada).
// A criação por linguagem natural (parsing por IA) entra na fase 2.
type alertInput struct {
	Message    string `json:"message"`
	FireAt     string `json:"fireAt"` // ISO 8601 (RFC3339) com offset
	Recurrence string `json:"recurrence"`
	NoteID     *int64 `json:"noteId"`
}

type snoozeInput struct {
	Minutes int `json:"minutes"`
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	alerts, err := s.core.Alerts.List(r.Context(), userID)
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
