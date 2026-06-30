package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// User é uma conta. PasswordHash é o hash bcrypt (nunca a senha em claro).
type User struct {
	ID           int64
	Email        string
	PasswordHash string
}

// CreateUser insere a conta e a linha default de preferências, numa transação.
// Email duplicado retorna erro do Postgres (unique violation) — o handler trata.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		email, passwordHash,
	).Scan(&id)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_prefs (user_id) VALUES ($1)`, id,
	); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return User{ID: id, Email: email, PasswordHash: passwordHash}, nil
}

// UserByEmail busca uma conta pelo email. Ausente => core.ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, core.ErrNotFound
	}
	return u, err
}

// UserTimezone retorna o fuso do usuário de user_prefs, ou 'UTC' se não definido.
func (s *Store) UserTimezone(ctx context.Context, userID int64) (string, error) {
	var tz string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(timezone,'UTC') FROM user_prefs WHERE user_id=$1`, userID,
	).Scan(&tz)
	if errors.Is(err, pgx.ErrNoRows) {
		return "UTC", nil
	}
	return tz, err
}

type UserPrefs struct {
	Model           string `json:"model"`
	Language        string `json:"language"`
	SystemPrompt    string `json:"systemPrompt"`
	CharLimit       int    `json:"charLimit"`
	ChatMaxTokens   int    `json:"chatMaxTokens"`
	Timezone        string `json:"timezone"`
	OpenRouterKey   string `json:"openrouterKey"`
	GCalSyncEnabled bool   `json:"gcalSyncEnabled"`
}

func (s *Store) GetUserPrefs(ctx context.Context, userID int64) (UserPrefs, error) {
	var p UserPrefs
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(model,''), COALESCE(language,''), COALESCE(system_prompt,''),
		        COALESCE(note_char_limit,0), COALESCE(chat_max_tokens,0),
		        COALESCE(timezone,'UTC'),
		        COALESCE(openrouter_key,''), COALESCE(gcal_sync_enabled,false)
		   FROM user_prefs WHERE user_id=$1`, userID,
	).Scan(&p.Model, &p.Language, &p.SystemPrompt, &p.CharLimit, &p.ChatMaxTokens, &p.Timezone, &p.OpenRouterKey, &p.GCalSyncEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserPrefs{}, nil
	}
	return p, err
}

// GetUserOpenRouterKey retorna a chave OpenRouter do usuário. Se não definida
// ou a linha de prefs não existir, retorna "" (sem erro).
func (s *Store) GetUserOpenRouterKey(ctx context.Context, userID int64) (string, error) {
	var key string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(openrouter_key,'') FROM user_prefs WHERE user_id=$1`, userID,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return key, err
}

func (s *Store) SetUserPrefs(ctx context.Context, userID int64, p UserPrefs) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_prefs (user_id, model, language, system_prompt, note_char_limit, chat_max_tokens, timezone, openrouter_key, gcal_sync_enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (user_id) DO UPDATE SET
		   model = EXCLUDED.model,
		   language = EXCLUDED.language,
		   system_prompt = EXCLUDED.system_prompt,
		   note_char_limit = EXCLUDED.note_char_limit,
		   chat_max_tokens = EXCLUDED.chat_max_tokens,
		   timezone = EXCLUDED.timezone,
		   openrouter_key = EXCLUDED.openrouter_key,
		   gcal_sync_enabled = EXCLUDED.gcal_sync_enabled`,
		userID, p.Model, p.Language, p.SystemPrompt, p.CharLimit, p.ChatMaxTokens, p.Timezone, p.OpenRouterKey, p.GCalSyncEnabled)
	return err
}
