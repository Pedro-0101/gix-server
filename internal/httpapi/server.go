// Package httpapi expõe o core via HTTP. É o adapter de canal do desktop/web:
// REST com auth por Bearer token (fase 1). Adapters de bots (WhatsApp/Telegram)
// virão como outros pacotes sobre o mesmo core, sem duplicar lógica.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/ratelimit"
	"github.com/Pedro-0101/gix-server/internal/store"
)

type Server struct {
	core  *core.Core
	auth  *auth.Authenticator
	users *store.Store
	push  *PushHub
	rl    *ratelimit.Store
}

// New monta o roteador. Rotas /v1/auth são públicas; o resto exige Bearer token.
// Rotas normais: rate=10 req/s burst=20; streaming (chat, push): rate=2 burst=5.
// corsOrigins lista as origens liberadas no CORS (ex.: ["*"]).
func New(c *core.Core, a *auth.Authenticator, users *store.Store, push *PushHub, corsOrigins []string) http.Handler {
	s := &Server{core: c, auth: a, users: users, push: push, rl: ratelimit.New()}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// públicas
	mux.HandleFunc("POST /v1/auth/signup", s.signup)
	mux.HandleFunc("POST /v1/auth/login", s.login)
	mux.HandleFunc("POST /v1/auth/refresh", s.refresh)

	// protegidas (Bearer token -> userID no contexto)
	protected := func(h http.HandlerFunc) http.Handler {
		return s.rateLimit(10, 20)(s.auth.Middleware(h))
	}
	streaming := func(h http.HandlerFunc) http.Handler {
		return s.rateLimit(2, 5)(s.auth.Middleware(h))
	}
	mux.Handle("GET /v1/notes", protected(s.listNotes))
	mux.Handle("POST /v1/notes", protected(s.createNote))
	mux.Handle("GET /v1/notes/graph", protected(s.graph))
	mux.Handle("POST /v1/notes/backfill-embeddings", protected(s.backfillEmbeddings))
	mux.Handle("GET /v1/notes/find", protected(s.findNotes))
	mux.Handle("GET /v1/notes/ask", protected(s.askNotes))
	mux.Handle("POST /v1/notes/capture", protected(s.captureNote))
	mux.Handle("GET /v1/notes/{id}", protected(s.getNote))
	mux.Handle("PUT /v1/notes/{id}", protected(s.updateNote))
	mux.Handle("DELETE /v1/notes/{id}", protected(s.deleteNote))
	mux.Handle("PUT /v1/notes/{id}/char-limit", protected(s.setCharLimit))
	mux.Handle("POST /v1/notes/{id}/summarize", protected(s.summarizeNote))
	mux.Handle("POST /v1/notes/{id}/tidy", protected(s.tidyNote))
	mux.Handle("POST /v1/notes/{id}/append", protected(s.appendToNote))
	mux.Handle("POST /v1/notes/{id}/overflow", protected(s.resolveOverflow))

	// alertas (CRUD + parsing por linguagem natural via IA)
	mux.Handle("GET /v1/alerts", protected(s.listAlerts))
	mux.Handle("POST /v1/alerts", protected(s.createAlert))
	mux.Handle("POST /v1/alerts/parse", protected(s.parseAlert))
	mux.Handle("POST /v1/alerts/{id}/done", protected(s.doneAlert))
	mux.Handle("POST /v1/alerts/{id}/cancel", protected(s.cancelAlert))
	mux.Handle("POST /v1/alerts/{id}/snooze", protected(s.snoozeAlert))
	mux.Handle("POST /v1/notes/{id}/alert", protected(s.createNoteAlert))

	// histórico de conversas
	mux.Handle("GET /v1/conversations", protected(s.listConversations))
	mux.Handle("GET /v1/conversations/{id}/messages", protected(s.conversationMessages))
	mux.Handle("DELETE /v1/conversations/{id}", protected(s.deleteConversation))

	// modelos disponíveis (da tabela de preços)
	mux.Handle("GET /v1/models", protected(s.listModels))

	// preferências do usuário (modelo, idioma, system_prompt, char_limit, timezone)
	mux.Handle("GET /v1/prefs", protected(s.getPrefs))
	mux.Handle("PUT /v1/prefs", protected(s.updatePrefs))

	// chat com IA: stream SSE — rate menor por ser conexão longa
	mux.Handle("POST /v1/chat", streaming(s.chatSend))

	// push de saída: stream SSE — rate menor por ser conexão longa
	mux.Handle("GET /v1/push", streaming(s.streamPush))

	return loggingMiddleware(corsMiddleware(corsOrigins)(mux))
}

// corsMiddleware libera chamadas cross-origin (o frontend desktop/web roda em
// outra origem que o gix-server) e responde ao preflight OPTIONS — que o mux
// roteado por método não atende sozinho. Auth é por Bearer token (sem cookies),
// então "*" é seguro; em produção restrinja via CORS_ALLOWED_ORIGINS.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || allowed[origin]) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimit retorna um middleware token bucket. Se o usuário exceder, 429.
func (s *Server) rateLimit(rate, burst int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, found := auth.UserID(r.Context())
			if !found {
				next.ServeHTTP(w, r)
				return
			}
			if !s.rl.Allow(userID, rate, burst) {
				http.Error(w, "muitas requisições", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware loga método, caminho e duração de cada request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
