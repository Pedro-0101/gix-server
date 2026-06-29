package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Pedro-0101/gix-server/internal/core"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token string `json:"token"`
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
	token, err := s.auth.Issue(user.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{Token: token})
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
	token, err := s.auth.Issue(user.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{Token: token})
}
