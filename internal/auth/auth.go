// Package auth cuida de senhas (bcrypt) e tokens de acesso (JWT). É o que os
// canais "com login" (desktop/web) usam; bots ligam identidade de outro jeito
// (fase 4) mas chegam ao mesmo userID escopado.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 24 * time.Hour // access token; refresh fica p/ depois

type ctxKey struct{}

// Authenticator assina/valida tokens com um segredo compartilhado.
type Authenticator struct {
	secret []byte
}

func New(secret string) *Authenticator {
	return &Authenticator{secret: []byte(secret)}
}

// Hash gera o hash bcrypt de uma senha.
func (a *Authenticator) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// Check confere a senha contra o hash.
func (a *Authenticator) Check(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Issue emite um token de acesso para o usuário.
func (a *Authenticator) Issue(userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
}

// parse valida o token e devolve o userID.
func (a *Authenticator) parse(token string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inesperado")
		}
		return a.secret, nil
	})
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

// Middleware exige um Bearer token válido e injeta o userID no contexto.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" || raw == r.Header.Get("Authorization") {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		userID, err := a.parse(strings.TrimSpace(raw))
		if err != nil {
			http.Error(w, "token inválido", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserID extrai o usuário autenticado do contexto.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxKey{}).(int64)
	return id, ok
}
