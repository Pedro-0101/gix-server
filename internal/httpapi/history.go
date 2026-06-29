package httpapi

import (
	"net/http"
	"strconv"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/service"
)

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	opts := service.ParseListOptions(limit, offset)
	convs, err := s.core.History.List(r.Context(), userID, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, convs)
}

func (s *Server) conversationMessages(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	msgs, err := s.core.History.Messages(r.Context(), userID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, id, ok := userAndID(w, r)
	if !ok {
		return
	}
	if err := s.core.History.Delete(r.Context(), userID, id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
