package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// ListNotes retorna as notas do usuário, mais novas primeiro.
func (s *Store) ListNotes(ctx context.Context, userID int64) ([]core.Note, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, content, tags, char_limit, created_at, updated_at
		   FROM notes WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotes(rows)
}

// GetNote retorna uma nota do usuário. Ausente/de outro dono => core.ErrNotFound.
func (s *Store) GetNote(ctx context.Context, userID, id int64) (core.Note, error) {
	var n core.Note
	err := s.pool.QueryRow(ctx,
		`SELECT id, title, content, tags, char_limit, created_at, updated_at
		   FROM notes WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CharLimit, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Note{}, core.ErrNotFound
	}
	return n, err
}

// CreateNote insere uma nota (sem IA/embedding — o vetor entra na fase 2).
func (s *Store) CreateNote(ctx context.Context, userID int64, title, content string, tags []string, charLimit int) (core.Note, error) {
	if tags == nil {
		tags = []string{}
	}
	var n core.Note
	err := s.pool.QueryRow(ctx,
		`INSERT INTO notes (user_id, title, content, tags, char_limit)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, title, content, tags, char_limit, created_at, updated_at`,
		userID, title, content, tags, charLimit,
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CharLimit, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

// UpdateNote salva título/corpo/tags e toca updated_at. Escopado por usuário.
func (s *Store) UpdateNote(ctx context.Context, userID, id int64, title, content string, tags []string) (core.Note, error) {
	if tags == nil {
		tags = []string{}
	}
	var n core.Note
	err := s.pool.QueryRow(ctx,
		`UPDATE notes SET title = $3, content = $4, tags = $5, updated_at = now()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, title, content, tags, char_limit, created_at, updated_at`,
		id, userID, title, content, tags,
	).Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CharLimit, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Note{}, core.ErrNotFound
	}
	return n, err
}

// DeleteNote remove a nota. Ausente => core.ErrNotFound.
func (s *Store) DeleteNote(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

// SetCharLimit define o override de limite por nota (0 = herda o global).
func (s *Store) SetCharLimit(ctx context.Context, userID, id int64, limit int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE notes SET char_limit = $3, updated_at = now() WHERE id = $1 AND user_id = $2`,
		id, userID, limit)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

// GraphNotes carrega id/título/tags de todas as notas do usuário (p/ o grafo).
func (s *Store) GraphNotes(ctx context.Context, userID int64) ([]core.Note, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, tags FROM notes WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Note
	for rows.Next() {
		var n core.Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Tags); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func scanNotes(rows pgx.Rows) ([]core.Note, error) {
	out := []core.Note{} // nunca nil: lista vazia serializa como [] (não null)
	for rows.Next() {
		var n core.Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CharLimit, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
