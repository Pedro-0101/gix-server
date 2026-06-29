package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamAccumulatesDeltas(t *testing.T) {
	sse := strings.Join([]string{
		": OPENROUTER PROCESSING",
		"",
		`data: {"choices":[{"delta":{"content":"Olá"}}]}`,
		`data: {"choices":[{"delta":{"content":", mundo"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"!"}}]}`,
		"data: [DONE]",
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := New("testkey")
	c.baseURL = srv.URL

	var got strings.Builder
	_, err := c.Stream(context.Background(), "m", []Message{{Role: "user", Content: "oi"}},
		func(s string) { got.WriteString(s) })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "Olá, mundo!" {
		t.Errorf("got %q, want %q", got.String(), "Olá, mundo!")
	}
}

func TestCompleteReturnsContentAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"role":"assistant","content":"{\"action\":\"create\"}"}}],
			"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}
		}`))
	}))
	defer srv.Close()

	c := New("testkey")
	c.baseURL = srv.URL

	content, usage, err := c.Complete(context.Background(), "m", []Message{{Role: "user", Content: "oi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if content != `{"action":"create"}` {
		t.Errorf("content = %q", content)
	}
	if usage == nil || usage.TotalTokens != 15 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestCompleteErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"chave invalida"}`))
	}))
	defer srv.Close()

	c := New("ruim")
	c.baseURL = srv.URL
	if _, _, err := c.Complete(context.Background(), "m", []Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("esperava erro, veio nil")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("erro deveria citar status 401: %v", err)
	}
}

func TestStreamErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"chave invalida"}`))
	}))
	defer srv.Close()

	c := New("ruim")
	c.baseURL = srv.URL
	_, err := c.Stream(context.Background(), "m", []Message{{Role: "user", Content: "x"}}, func(string) {})
	if err == nil {
		t.Fatal("esperava erro, veio nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("erro deveria citar status 401: %v", err)
	}
}

func TestStreamCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New("k")
	c.baseURL = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Stream(ctx, "m", []Message{{Role: "user", Content: "x"}}, func(string) {}); err == nil {
		t.Fatal("esperava erro de contexto cancelado")
	}
}

func TestStreamCancelledMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			if r.Context().Err() != nil {
				return
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n"))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := New("k")
	c.baseURL = srv.URL
	_, err := c.Stream(ctx, "m", []Message{{Role: "user", Content: "x"}}, func(s string) {
		cancel() // cancela ao receber o primeiro delta (mid-stream)
	})
	if err == nil {
		t.Fatal("esperava erro de cancelamento mid-stream")
	}
}

func TestStreamToolsAccumulatesToolCallDeltas(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"create_alert","arguments":"{\"message\":\"x\","}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"fire_at\":\"2099-01-01T09:00:00-03:00\"}"}}]}}]}`,
		`data: {"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		"data: [DONE]",
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := New("k")
	c.baseURL = srv.URL

	var text strings.Builder
	usage, calls, err := c.StreamTools(context.Background(), "m",
		[]Message{{Role: "user", Content: "oi"}}, []Tool{{Type: "function"}},
		func(s string) { text.WriteString(s) })
	if err != nil {
		t.Fatalf("StreamTools: %v", err)
	}
	if text.String() != "" {
		t.Fatalf("não deveria haver texto, veio %q", text.String())
	}
	if len(calls) != 1 || calls[0].Name != "create_alert" {
		t.Fatalf("esperava 1 tool call create_alert, veio %+v", calls)
	}
	if calls[0].Arguments != `{"message":"x","fire_at":"2099-01-01T09:00:00-03:00"}` {
		t.Fatalf("arguments concatenados errados: %q", calls[0].Arguments)
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestStreamToolsMultipleToolCalls(t *testing.T) {
	sse := strings.Join([]string{
		// index 0 começa (nome + início dos args)
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"create_alert","arguments":"{\"message\":\"a\","}}]}}]}`,
		// index 1 começa (nome + args completos numa só delta)
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"name":"create_note","arguments":"{\"title\":\"b\"}"}}]}}]}`,
		// index 0 termina (resto dos args, split em duas deltas)
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"fire_at\":\"2099-01-01T09:00:00-03:00\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}`,
		`data: {"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		"data: [DONE]",
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := New("k")
	c.baseURL = srv.URL

	var text strings.Builder
	_, calls, err := c.StreamTools(context.Background(), "m",
		[]Message{{Role: "user", Content: "oi"}}, []Tool{{Type: "function"}},
		func(s string) { text.WriteString(s) })
	if err != nil {
		t.Fatalf("StreamTools: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("esperava 2 tool calls, veio %+v", calls)
	}
	if calls[0].Name != "create_alert" {
		t.Fatalf("call[0].Name = %q, esperava create_alert", calls[0].Name)
	}
	if calls[0].Arguments != `{"message":"a","fire_at":"2099-01-01T09:00:00-03:00"}` {
		t.Fatalf("call[0].Arguments = %q", calls[0].Arguments)
	}
	if calls[1].Name != "create_note" {
		t.Fatalf("call[1].Name = %q, esperava create_note", calls[1].Name)
	}
	if calls[1].Arguments != `{"title":"b"}` {
		t.Fatalf("call[1].Arguments = %q", calls[1].Arguments)
	}
}
