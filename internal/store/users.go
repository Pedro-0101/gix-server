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
