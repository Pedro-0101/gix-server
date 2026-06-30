package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

type GoogleToken struct {
	UserID       int64
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func (s *Store) UpsertGoogleToken(ctx context.Context, userID int64, accessToken, refreshToken string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO google_tokens (user_id, access_token, refresh_token, expires_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET
		   access_token = EXCLUDED.access_token,
		   refresh_token = EXCLUDED.refresh_token,
		   expires_at = EXCLUDED.expires_at`,
		userID, accessToken, refreshToken, expiresAt)
	return err
}

func (s *Store) GetGoogleToken(ctx context.Context, userID int64) (GoogleToken, error) {
	var t GoogleToken
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, access_token, refresh_token, expires_at
		   FROM google_tokens WHERE user_id = $1`, userID,
	).Scan(&t.UserID, &t.AccessToken, &t.RefreshToken, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoogleToken{}, core.ErrNotFound
	}
	return t, err
}

func (s *Store) DeleteGoogleToken(ctx context.Context, userID int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM google_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}
