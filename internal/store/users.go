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
	Model        string `json:"model"`
	Language     string `json:"language"`
	SystemPrompt string `json:"systemPrompt"`
	CharLimit    int    `json:"charLimit"`
	Timezone     string `json:"timezone"`
}

func (s *Store) GetUserPrefs(ctx context.Context, userID int64) (UserPrefs, error) {
	var p UserPrefs
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(model,''), COALESCE(language,''), COALESCE(system_prompt,''),
		        COALESCE(note_char_limit,0), COALESCE(timezone,'UTC')
		   FROM user_prefs WHERE user_id=$1`, userID,
	).Scan(&p.Model, &p.Language, &p.SystemPrompt, &p.CharLimit, &p.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserPrefs{}, nil
	}
	return p, err
}

func (s *Store) SetUserPrefs(ctx context.Context, userID int64, p UserPrefs) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_prefs (user_id, model, language, system_prompt, note_char_limit, timezone)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id) DO UPDATE SET
		   model = EXCLUDED.model,
		   language = EXCLUDED.language,
		   system_prompt = EXCLUDED.system_prompt,
		   note_char_limit = EXCLUDED.note_char_limit,
		   timezone = EXCLUDED.timezone`,
		userID, p.Model, p.Language, p.SystemPrompt, p.CharLimit, p.Timezone)
	return err
}
