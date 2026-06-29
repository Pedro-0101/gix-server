// Package config carrega a configuração do servidor a partir do ambiente.
// Em dev, lê um .env (best-effort); em produção (Railway) usa as variáveis do
// serviço.
package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultModel é o modelo de IA usado quando AI_MODEL não é definido. Toda IA
// roda no servidor via OpenRouter; o modelo é um só por enquanto (seleção por
// usuário/provider fica para depois).
const DefaultModel = "google/gemini-2.5-flash-lite"

type Config struct {
	Port          string // injetado pela Railway; default 8080
	DatabaseURL   string // Postgres (pgvector habilitado)
	JWTSecret     string // assina os tokens de auth
	OpenRouterKey string // chave da IA — vive só no servidor (usada na fase 2)
	AIModel       string // modelo OpenRouter usado em todas as intents de IA
	EmbedModelPath string // diretório local do modelo ONNX (default "data/embed-model")
	CORSOrigins   []string // origens permitidas no CORS; default "*" (libera todas, ok p/ dev)
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
		AIModel:        getenv("AI_MODEL", DefaultModel),
		EmbedModelPath: getenv("EMBED_MODEL_PATH", "data/embed-model"),
		CORSOrigins:    splitOrigins(getenv("CORS_ALLOWED_ORIGINS", "*")),
	}
}

// splitOrigins parte a lista de origens separadas por vírgula, descartando
// espaços e entradas vazias.
func splitOrigins(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
