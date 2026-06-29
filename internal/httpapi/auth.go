package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

// tokenResponse devolve o par de tokens: o access (JWT curto, em todo request)
// e o refresh (opaco longo, só p/ renovar via /v1/auth/refresh).
type tokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// issueTokens emite o par access+refresh para o usuário e persiste o hash do
// refresh. Responde direto em caso de erro; em sucesso devolve o par.
func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, userID int64) (tokenResponse, bool) {
	access, err := s.auth.Issue(userID)
	if err != nil {
		writeErr(w, err)
		return tokenResponse{}, false
	}
	raw, hash, expiresAt, err := s.auth.NewRefreshToken()
	if err != nil {
		writeErr(w, err)
		return tokenResponse{}, false
	}
	if err := s.users.CreateRefreshToken(r.Context(), userID, hash, expiresAt); err != nil {
		writeErr(w, err)
		return tokenResponse{}, false
	}
	return tokenResponse{AccessToken: access, RefreshToken: raw}, true
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || len(in.Password) < 8 {
		http.Error(w, "email e senha (>=8 caracteres) obrigatórios", http.StatusBadRequest)
		return
	}
	if _, err := s.users.UserByEmail(r.Context(), in.Email); err == nil {
		http.Error(w, "email já cadastrado", http.StatusConflict)
		return
	} else if !errors.Is(err, core.ErrNotFound) {
		writeErr(w, err)
		return
	}

	hash, err := s.auth.Hash(in.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	user, err := s.users.CreateUser(r.Context(), in.Email, hash)
	if err != nil {
		writeErr(w, err)
		return
	}
	tokens, ok := s.issueTokens(w, r, user.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, tokens)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))

	user, err := s.users.UserByEmail(r.Context(), in.Email)
	if errors.Is(err, core.ErrNotFound) || (err == nil && !s.auth.Check(user.PasswordHash, in.Password)) {
		http.Error(w, "credenciais inválidas", http.StatusUnauthorized)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	tokens, ok := s.issueTokens(w, r, user.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

// refresh troca um refresh token válido por um novo par (rotação). Token
// inválido/expirado/já usado => 401.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var in refreshInput
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	if in.RefreshToken == "" {
		http.Error(w, "refreshToken obrigatório", http.StatusBadRequest)
		return
	}
	userID, err := s.users.ConsumeRefreshToken(r.Context(), auth.HashRefreshToken(in.RefreshToken))
	if errors.Is(err, core.ErrNotFound) {
		http.Error(w, "refresh token inválido", http.StatusUnauthorized)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	tokens, ok := s.issueTokens(w, r, userID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}
