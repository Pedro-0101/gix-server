package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// mockCore returns a core.Core with no-op intents (nil pool safe for auth-only
// tests). Tests that hit the store skip when DATABASE_URL is unset.
func mockCore() *core.Core {
	return &core.Core{}
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(New(nil, nil, nil, nil, []string{"*"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", resp.StatusCode)
	}
}

func TestAuthSignupNoBody(t *testing.T) {
	srv := httptest.NewServer(New(mockCore(), auth.New("test-secret-12345678"), &store.Store{}, NewPushHub(&store.Store{}), []string{"*"}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/auth/signup", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/auth/signup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperava 400", resp.StatusCode)
	}
}

func TestAuthLoginNoBody(t *testing.T) {
	srv := httptest.NewServer(New(mockCore(), auth.New("test-secret-12345678"), &store.Store{}, NewPushHub(&store.Store{}), []string{"*"}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/auth/login", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/auth/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperava 400", resp.StatusCode)
	}
}

func TestProtectedRouteWithoutToken(t *testing.T) {
	srv := httptest.NewServer(New(mockCore(), auth.New("test-secret-12345678"), &store.Store{}, NewPushHub(&store.Store{}), []string{"*"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/notes")
	if err != nil {
		t.Fatalf("GET /v1/notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperava 401", resp.StatusCode)
	}
}

func TestRefreshWithoutToken(t *testing.T) {
	srv := httptest.NewServer(New(mockCore(), auth.New("test-secret-12345678"), &store.Store{}, NewPushHub(&store.Store{}), []string{"*"}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/auth/refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/auth/refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperava 400", resp.StatusCode)
	}
}
