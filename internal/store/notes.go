package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// ListNotes retorna as notas do usuário, mais novas primeiro.
func (s *Store) ListNotes(ctx context.Context, userID int64, p Pagination) ([]core.Note, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, content, tags, char_limit, created_at, updated_at
		   FROM notes WHERE user_id = $1 ORDER BY created_at DESC
		   LIMIT $2 OFFSET $3`, userID, p.Limit, p.Offset)
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

// UpdateNoteEmbedding salva o embedding vector(384) de uma nota.
func (s *Store) UpdateNoteEmbedding(ctx context.Context, userID, noteID int64, emb []float32) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE notes SET embedding = $3 WHERE id = $1 AND user_id = $2`,
		noteID, userID, pgvector.NewVector(emb))
	return err
}

// NotesWithoutEmbedding retorna até limit notas cujo embedding é NULL.
type NoteStub struct {
	ID      int64
	UserID  int64
	Title   string
	Content string
}

func (s *Store) NotesWithoutEmbedding(ctx context.Context, limit int) ([]NoteStub, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, title, content FROM notes
		  WHERE embedding IS NULL
		  ORDER BY id ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NoteStub
	for rows.Next() {
		var ns NoteStub
		if err := rows.Scan(&ns.ID, &ns.UserID, &ns.Title, &ns.Content); err != nil {
			return nil, err
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// SearchHit é um resultado da busca FTS ou vetorial (nota + score).
type SearchHit struct {
	NoteID int64
	Score  float64
}

// SearchFTS busca notas por texto completo (tsvector + ts_rank), escopado por
// usuário. Retorna até limit resultados ordenados por relevância (melhor primeiro).
func (s *Store) SearchFTS(ctx context.Context, userID int64, query string, limit int) ([]SearchHit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ts_rank(fts, plainto_tsquery('portuguese', unaccent($3))) AS rank
		   FROM notes
		  WHERE user_id = $1 AND fts @@ plainto_tsquery('portuguese', unaccent($3))
		  ORDER BY rank DESC
		  LIMIT $2`, userID, limit, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.NoteID, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchVector busca notas por similaridade de cosseno (pgvector), escopado por
// usuário. Retorna até limit resultados ordenados por similaridade (melhor primeiro).
func (s *Store) SearchVector(ctx context.Context, userID int64, emb []float32, limit int) ([]SearchHit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, 1 - (embedding <=> $3::vector) AS sim
		   FROM notes
		  WHERE user_id = $1 AND embedding IS NOT NULL
		  ORDER BY embedding <=> $3::vector
		  LIMIT $2`, userID, limit, pgvector.NewVector(emb))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.NoteID, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// NotesByIDs retorna as notas do usuário que correspondem aos ids, na ordem
// solicitada. Notas não encontradas ou de outro usuário são omitidas.
func (s *Store) NotesByIDs(ctx context.Context, userID int64, ids []int64) ([]core.Note, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, content, tags, char_limit, created_at, updated_at
		   FROM notes WHERE id = ANY($1) AND user_id = $2`,
		ids, userID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]core.Note{}
	for rows.Next() {
		var n core.Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Tags, &n.CharLimit, &n.CreatedAt, &n.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		byID[n.ID] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]core.Note, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
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
