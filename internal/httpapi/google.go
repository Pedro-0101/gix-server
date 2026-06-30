package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
)

type gcalState struct {
	UserID int64  `json:"u"`
	Nonce  string `json:"n"`
	MAC    string `json:"m"`
}

func signState(userID int64, secret string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(b)
	payload := gcalState{UserID: userID, Nonce: nonce}

	mac := hmac.New(sha256.New, []byte(secret))
	data, _ := json.Marshal(payload)
	mac.Write(data)
	payload.MAC = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func verifyState(state, secret string) (int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return 0, err
	}
	var payload gcalState
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return 0, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	payloadWithoutMAC := gcalState{UserID: payload.UserID, Nonce: payload.Nonce}
	data, _ := json.Marshal(payloadWithoutMAC)
	mac.Write(data)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(payload.MAC), []byte(expected)) {
		return 0, errors.New("state MAC inválido")
	}
	return payload.UserID, nil
}

func (s *Server) googleAuthURL(w http.ResponseWriter, r *http.Request) {
	if s.gcal == nil || !s.gcal.IsConfigured() {
		http.Error(w, "Google Calendar não configurado no servidor", http.StatusServiceUnavailable)
		return
	}
	userID, _ := auth.UserID(r.Context())
	state, err := signState(userID, s.auth.Secret())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": s.gcal.AuthURL(state)})
}

func (s *Server) googleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.gcal == nil || !s.gcal.IsConfigured() {
		writeGCalErrorHTML(w, "Google Calendar não configurado no servidor")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		writeGCalErrorHTML(w, "Requisição inválida — state ausente")
		return
	}
	userID, err := verifyState(state, s.auth.Secret())
	if err != nil {
		writeGCalErrorHTML(w, "State inválido ou expirado. Refaça a autenticação.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeGCalErrorHTML(w, "Requisição inválida — code ausente")
		return
	}

	token, err := s.gcal.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("gcal: exchange", "err", err)
		writeGCalErrorHTML(w, "Falha ao autenticar com Google. Verifique as credenciais.")
		return
	}

	if err := s.users.UpsertGoogleToken(r.Context(), userID, token.AccessToken, token.RefreshToken, token.ExpiresAt); err != nil {
		slog.Error("gcal: upsert token", "err", err)
		writeGCalErrorHTML(w, "Falha ao salvar token. Tente novamente.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(gcalSuccessHTML))
}

func (s *Server) googleAuthStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	_, err := s.users.GetGoogleToken(r.Context(), userID)
	connected := err == nil

	prefs, prefsErr := s.users.GetUserPrefs(r.Context(), userID)
	syncEnabled := false
	if prefsErr == nil {
		syncEnabled = prefs.GCalSyncEnabled
	}

	writeJSON(w, http.StatusOK, map[string]bool{"connected": connected, "syncEnabled": syncEnabled})
}

func (s *Server) googleAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	if err := s.users.DeleteGoogleToken(r.Context(), userID); err != nil && !errors.Is(err, core.ErrNotFound) {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const gcalSuccessHTML = `<!DOCTYPE html>
<html lang="pt">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Google Calendar Conectado</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display:flex; align-items:center; justify-content:center; min-height:100vh; background:#f5f5f5; color:#1a1a1a; }
  .card { background:#fff; border-radius:12px; padding:48px 40px; text-align:center; max-width:420px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); }
  .check { width:64px; height:64px; border-radius:50%; background:#34a853; display:inline-flex; align-items:center; justify-content:center; margin-bottom:24px; }
  .check svg { width:32px; height:32px; stroke:white; stroke-width:3; fill:none; }
  h1 { font-size:20px; font-weight:600; margin-bottom:8px; }
  p { font-size:14px; color:#666; line-height:1.5; }
</style>
</head>
<body>
<div class="card">
  <div class="check"><svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg></div>
  <h1>Google Calendar conectado!</h1>
  <p>Sua conta Google foi vinculada com sucesso. Os alertas criados no gix agora serão sincronizados automaticamente com seu Google Calendar.</p>
</div>
<script>window.close()</script>
</body>
</html>`

const gcalErrorHTMLFmt = `<!DOCTYPE html>
<html lang="pt">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Erro na conexão</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display:flex; align-items:center; justify-content:center; min-height:100vh; background:#f5f5f5; color:#1a1a1a; }
  .card { background:#fff; border-radius:12px; padding:48px 40px; text-align:center; max-width:420px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); }
  .x { width:64px; height:64px; border-radius:50%; background:#ea4335; display:inline-flex; align-items:center; justify-content:center; margin-bottom:24px; }
  .x svg { width:32px; height:32px; stroke:white; stroke-width:3; fill:none; }
  h1 { font-size:20px; font-weight:600; margin-bottom:8px; }
  p { font-size:14px; color:#666; line-height:1.5; }
</style>
</head>
<body>
<div class="card">
  <div class="x"><svg viewBox="0 0 24 24"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></div>
  <h1>Falha na conexão</h1>
  <p>%s</p>
</div>
</body>
</html>`

func writeGCalErrorHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(gcalErrorHTMLFmt, msg)))
}
