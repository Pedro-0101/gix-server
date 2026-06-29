package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// CreateRefreshToken guarda o HASH de um refresh token recém-emitido.
func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
	return err
}

// ConsumeRefreshToken valida e ROTACIONA um refresh token numa transação:
// confere que existe, não foi revogado e não expirou; marca como revogado e
// devolve o userID dono. Token inválido/expirado/já usado => core.ErrNotFound
// (o handler traduz p/ 401). A rotação garante uso único.
func (s *Store) ConsumeRefreshToken(ctx context.Context, tokenHash string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	var userID int64
	var expiresAt time.Time
	var revoked bool
	err = tx.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked FROM refresh_tokens
		   WHERE token_hash = $1 FOR UPDATE`, tokenHash,
	).Scan(&userID, &expiresAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if revoked || time.Now().After(expiresAt) {
		return 0, core.ErrNotFound
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked = true WHERE token_hash = $1`, tokenHash); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return userID, nil
}
