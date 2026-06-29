// Comando server: ponto de entrada HTTP do gix-server.
//
// Liga config -> store (Postgres) -> core (intents) -> httpapi (REST + auth).
// Fase 1: auth + CRUD de notas funcionam ponta a ponta; as intents de IA/
// embeddings respondem 501 até a fase 2.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "time/tzdata" // tz embutido: LoadLocation funciona na imagem sem tzdata do SO

	"github.com/Pedro-0101/gix-server/internal/auth"
	"github.com/Pedro-0101/gix-server/internal/config"
	"github.com/Pedro-0101/gix-server/internal/httpapi"
	"github.com/Pedro-0101/gix-server/internal/scheduler"
	"github.com/Pedro-0101/gix-server/internal/service"
	"github.com/Pedro-0101/gix-server/internal/store"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL é obrigatório")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET é obrigatório")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	// push hub (SSE) é o Notifier do scheduler e o transporte da rota /v1/push.
	hub := httpapi.NewPushHub(st)
	go scheduler.New(st, hub).Run(ctx)

	handler := httpapi.New(service.NewCore(st), auth.New(cfg.JWTSecret), st, hub)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("gix-server ouvindo em :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
