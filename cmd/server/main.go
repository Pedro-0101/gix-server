// Comando server: ponto de entrada HTTP do gix-server.
//
// Liga config -> store (Postgres) -> core (intents) -> httpapi (REST + auth).
// Fase 1: auth + CRUD de notas funcionam ponta a ponta; as intents de IA/
// embeddings respondem 501 até a fase 2.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "time/tzdata" // tz embutido: LoadLocation funciona na imagem sem tzdata do SO

	"github.com/Pedro-0101/gix-server/internal/ai"
	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/config"
	"github.com/Pedro-0101/gix-server/internal/embed"
	"github.com/Pedro-0101/gix-server/internal/httpapi"
	"github.com/Pedro-0101/gix-server/internal/scheduler"
	"github.com/Pedro-0101/gix-server/internal/service"
	"github.com/Pedro-0101/gix-server/internal/store"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL é obrigatório")
		os.Exit(1)
	}
	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET é obrigatório")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// push hub (SSE) é o Notifier do scheduler e o transporte da rota /v1/push.
	hub := httpapi.NewPushHub(st)
	go scheduler.New(st, hub).Run(ctx)

	// embedder ONNX (embeddings server-side). Se falhar (CGO/lib ausente), segue
	// sem — notas criadas/editadas ficam sem vetor até o deploy corrigir.
	var embedder *embed.Embedder
	if err := embed.EnsureModel(cfg.EmbedModelPath); err != nil {
		slog.Warn("embed: modelo não disponível, embeddings desativados", "err", err)
	} else {
		embedder, err = embed.NewEmbedder(cfg.EmbedModelPath)
		if err != nil {
			slog.Warn("embed: falha ao iniciar, embeddings desativados", "err", err)
		}
	}

	aiDeps := service.AI{
		Client:   ai.New(cfg.OpenRouterKey),
		Model:    cfg.AIModel,
		Embedder: embedder,
	}
	handler := httpapi.New(service.NewCore(st, aiDeps), auth.New(cfg.JWTSecret), st, hub, cfg.CORSOrigins)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("gix-server ouvindo", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("shutdown", "err", err)
	}
}
