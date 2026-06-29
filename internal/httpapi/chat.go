package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
)

type chatInput struct {
	ConversationID int64  `json:"conversationId"`
	Text           string `json:"text"`
}

// chatSend é o POST /v1/chat: stream SSE com as respostas da IA.
func (s *Server) chatSend(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var in chatInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	if in.Text == "" {
		http.Error(w, "texto vazio", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sink := func(ev core.ChatEvent) error {
		return writeChatSSE(w, flusher, ev)
	}

	err := s.core.Chat.Send(r.Context(), userID, core.ChatInput{
		ConversationID: in.ConversationID,
		Text:           in.Text,
	}, sink)
	if err != nil {
		writeChatSSE(w, flusher, core.ChatEvent{Type: "error", Err: err.Error()}) //nolint:errcheck
	}
}

// writeChatSSE serializa um ChatEvent como SSE e faz flush. Devolve erro se o
// cliente desconectou.
func writeChatSSE(w http.ResponseWriter, flusher http.Flusher, ev core.ChatEvent) error {
	switch ev.Type {
	case "delta":
		_, err := fmt.Fprintf(w, "event: delta\ndata: %s\n\n", jsonString(ev.Delta))
		if err != nil {
			return err
		}
	case "done":
		_, err := fmt.Fprintf(w, "event: done\ndata: %s\n\n", jsonString(ev.Content))
		if err != nil {
			return err
		}
	case "error":
		_, err := fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString(ev.Err))
		if err != nil {
			return err
		}
	case "usage":
		data, _ := json.Marshal(ev.Usage)
		_, err := fmt.Fprintf(w, "event: usage\ndata: %s\n\n", data)
		if err != nil {
			return err
		}
	case "note_proposed":
		data, _ := json.Marshal(ev.Note)
		_, err := fmt.Fprintf(w, "event: note_proposed\ndata: %s\n\n", data)
		if err != nil {
			return err
		}
	case "alert_proposed":
		data, _ := json.Marshal(ev.Alert)
		_, err := fmt.Fprintf(w, "event: alert_proposed\ndata: %s\n\n", data)
		if err != nil {
			return err
		}
	default:
		return nil
	}
	flusher.Flush()
	return nil
}

// jsonString retorna s como uma string JSON (escapada), para usar inline no
// campo data: do SSE sem precisar de um struct.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
