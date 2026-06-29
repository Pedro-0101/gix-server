// Package config carrega a configuração do servidor a partir do ambiente.
// Em dev, lê um .env (best-effort); em produção (Railway) usa as variáveis do
// serviço.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string // injetado pela Railway; default 8080
	DatabaseURL   string // Postgres (pgvector habilitado)
	JWTSecret     string // assina os tokens de auth
	OpenRouterKey string // chave da IA — vive só no servidor (usada na fase 2)
}

// Load lê o ambiente. Não valida campos obrigatórios — quem precisa decide
// (ex.: main exige DATABASE_URL e JWT_SECRET).
func Load() Config {
	_ = godotenv.Load() // sem .env? segue com o ambiente do processo.
	return Config{
		Port:          getenv("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
