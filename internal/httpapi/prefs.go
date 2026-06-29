package httpapi

import (
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/auth"
)

func (s *Server) getPrefs(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	prefs, err := s.users.GetUserPrefs(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

func (s *Server) updatePrefs(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var in struct {
		Model        *string `json:"model"`
		Language     *string `json:"language"`
		SystemPrompt *string `json:"systemPrompt"`
		CharLimit    *int    `json:"charLimit"`
		Timezone     *string `json:"timezone"`
	}
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	current, err := s.users.GetUserPrefs(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if in.Model != nil {
		current.Model = *in.Model
	}
	if in.Language != nil {
		current.Language = *in.Language
	}
	if in.SystemPrompt != nil {
		current.SystemPrompt = *in.SystemPrompt
	}
	if in.CharLimit != nil {
		current.CharLimit = *in.CharLimit
	}
	if in.Timezone != nil {
		current.Timezone = *in.Timezone
	}
	if err := s.users.SetUserPrefs(r.Context(), userID, current); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, current)
}
