package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// heartbeat mantém a conexão SSE viva (e detecta cliente morto na escrita).
const heartbeat = 25 * time.Second

// PushHub é o transporte de push pro desktop: um registro de conexões SSE ao
// vivo por usuário. Implementa scheduler.Notifier — o scheduler chama Deliver
// sem saber que por baixo é SSE (amanhã pode ser WhatsApp/Telegram). A entrega
// ao vivo é best-effort; a garantia de não-perda mora no outbox (store).
type PushHub struct {
	store  *store.Store
	mu     sync.Mutex
	nextID int
	conns  map[int64]map[int]chan store.Delivery
}

func NewPushHub(s *store.Store) *PushHub {
	return &PushHub{store: s, conns: map[int64]map[int]chan store.Delivery{}}
}

// Deliver faz fan-out de um disparo p/ as conexões vivas do usuário, sem
// bloquear: se o buffer do cliente está cheio (lento), pula — a entrega segue
// pendente no outbox e é reenviada no próximo connect.
func (h *PushHub) Deliver(userID int64, d store.Delivery) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.conns[userID] {
		select {
		case ch <- d:
		default:
		}
	}
}

func (h *PushHub) register(userID int64) (int, chan store.Delivery) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan store.Delivery, 16)
	if h.conns[userID] == nil {
		h.conns[userID] = map[int]chan store.Delivery{}
	}
	h.conns[userID][id] = ch
	return id, ch
}

func (h *PushHub) unregister(userID int64, id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.conns[userID]; m != nil {
		delete(m, id)
		if len(m) == 0 {
			delete(h.conns, userID)
		}
	}
}

// streamPush é o GET /v1/push: um stream SSE de longa duração. Registra a
// conexão ANTES do flush das pendentes p/ não perder um disparo que aconteça no
// meio (no pior caso, o cliente vê uma duplicata — MarkDelivered é idempotente).
func (s *Server) streamPush(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
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

	ctx := r.Context()
	id, ch := s.push.register(userID)
	defer s.push.unregister(userID, id)

	// catch-up: tudo que disparou enquanto o cliente estava offline.
	pending, err := s.push.store.UndeliveredDeliveries(ctx, userID)
	if err == nil {
		for _, d := range pending {
			if !s.sendDelivery(w, flusher, d) {
				return
			}
		}
	}

	ping := time.NewTicker(heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-ch:
			if !s.sendDelivery(w, flusher, d) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// sendDelivery escreve uma entrega como evento SSE e a marca entregue. Devolve
// false se a escrita falhou (cliente desconectou) — aí o caller encerra.
func (s *Server) sendDelivery(w http.ResponseWriter, flusher http.Flusher, d store.Delivery) bool {
	b, err := json.Marshal(d)
	if err != nil {
		return true // entrega corrompida não derruba o stream
	}
	if _, err := fmt.Fprintf(w, "event: alert\ndata: %s\n\n", b); err != nil {
		return false
	}
	flusher.Flush()
	_ = s.push.store.MarkDelivered(context.Background(), d.ID)
	return true
}
