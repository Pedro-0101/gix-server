package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// ListConversations retorna as conversas do usuário, mais novas primeiro. Nunca nil.
func (s *Store) ListConversations(ctx context.Context, userID int64, p Pagination) ([]core.Conversation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, model, created_at
		   FROM conversations WHERE user_id = $1
		   ORDER BY created_at DESC, id DESC
		   LIMIT $2 OFFSET $3`, userID, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []core.Conversation{}
	for rows.Next() {
		var c core.Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.Model, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetMessages retorna as mensagens de uma conversa do usuário, em ordem. A posse
// é checada por user_id na conversa; conversa inexistente/de outro dono =>
// core.ErrNotFound (em vez de devolver uma lista vazia ambígua).
func (s *Store) GetMessages(ctx context.Context, userID, conversationID int64) ([]core.Message, error) {
	if err := s.assertConversationOwner(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, role, content FROM messages
		   WHERE conversation_id = $1 ORDER BY id ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []core.Message{}
	for rows.Next() {
		var m core.Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteConversation remove a conversa do usuário; as mensagens caem por
// ON DELETE CASCADE. Inexistente/de outro dono => core.ErrNotFound.
func (s *Store) DeleteConversation(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

// CreateConversation cria uma nova conversa e retorna seu ID.
func (s *Store) CreateConversation(ctx context.Context, userID int64, title, model string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title, model) VALUES ($1, $2, $3) RETURNING id`,
		userID, title, model).Scan(&id)
	return id, err
}

// AddMessage insere uma mensagem em uma conversa.
func (s *Store) AddMessage(ctx context.Context, conversationID int64, role, content string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO messages (conversation_id, role, content) VALUES ($1, $2, $3)`,
		conversationID, role, content)
	return err
}

// assertConversationOwner confirma que a conversa existe e é do usuário.
func (s *Store) assertConversationOwner(ctx context.Context, userID, conversationID int64) error {
	var one int
	err := s.pool.QueryRow(ctx,
		`SELECT 1 FROM conversations WHERE id = $1 AND user_id = $2`, conversationID, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	return err
}
