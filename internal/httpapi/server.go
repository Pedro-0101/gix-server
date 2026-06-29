// Package httpapi expõe o core via HTTP. É o adapter de canal do desktop/web:
// REST com auth por Bearer token (fase 1). Adapters de bots (WhatsApp/Telegram)
// virão como outros pacotes sobre o mesmo core, sem duplicar lógica.
package httpapi

import (
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

type Server struct {
	core  *core.Core
	auth  *auth.Authenticator
	users *store.Store
}

// New monta o roteador. Rotas /v1/auth são públicas; o resto exige Bearer token.
func New(c *core.Core, a *auth.Authenticator, users *store.Store) http.Handler {
	s := &Server{core: c, auth: a, users: users}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// públicas
	mux.HandleFunc("POST /v1/auth/signup", s.signup)
	mux.HandleFunc("POST /v1/auth/login", s.login)

	// protegidas (Bearer token -> userID no contexto)
	protected := func(h http.HandlerFunc) http.Handler { return s.auth.Middleware(h) }
	mux.Handle("GET /v1/notes", protected(s.listNotes))
	mux.Handle("POST /v1/notes", protected(s.createNote))
	mux.Handle("GET /v1/notes/graph", protected(s.graph)) // literal vence o {id}
	mux.Handle("GET /v1/notes/{id}", protected(s.getNote))
	mux.Handle("PUT /v1/notes/{id}", protected(s.updateNote))
	mux.Handle("DELETE /v1/notes/{id}", protected(s.deleteNote))
	mux.Handle("PUT /v1/notes/{id}/char-limit", protected(s.setCharLimit))

	return mux
}
