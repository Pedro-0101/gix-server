package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// alertCols é a ordem canônica de leitura de um alerta.
const alertCols = "id, message, note_id, fire_at, recurrence, status, created_at"

// ListAlerts retorna os alertas do usuário, mais cedo primeiro. Nunca nil.
func (s *Store) ListAlerts(ctx context.Context, userID int64) ([]core.Alert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+alertCols+` FROM alerts
		   WHERE user_id = $1 ORDER BY fire_at ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlerts(rows)
}

// GetAlert retorna um alerta do usuário. Ausente/de outro dono => core.ErrNotFound.
func (s *Store) GetAlert(ctx context.Context, userID, id int64) (core.Alert, error) {
	a, err := scanAlert(s.pool.QueryRow(ctx,
		`SELECT `+alertCols+` FROM alerts WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Alert{}, core.ErrNotFound
	}
	return a, err
}

// CreateAlert insere um lembrete já estruturado (mensagem + fire_at + recorrência),
// vindo de uma proposta confirmada. O parsing por IA de linguagem natural fica
// no service (fase 2); aqui é CRUD puro. fire_at é gravado em UTC pelo Postgres.
func (s *Store) CreateAlert(ctx context.Context, a core.Alert, userID int64) (core.Alert, error) {
	created, err := scanAlert(s.pool.QueryRow(ctx,
		`INSERT INTO alerts (user_id, message, note_id, fire_at, recurrence)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+alertCols,
		userID, a.Message, a.NoteID, a.FireAt, a.Recurrence))
	return created, err
}

// SetAlertStatus muda o status (pending|done|cancelled) de um alerta do usuário.
// Linha inexistente/de outro dono => core.ErrNotFound.
func (s *Store) SetAlertStatus(ctx context.Context, userID, id int64, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE alerts SET status = $3 WHERE id = $1 AND user_id = $2`, id, userID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

// UpdateAlertFireAt reagenda um alerta (snooze, e depois recorrência no scheduler),
// mantendo-o pendente. Linha inexistente/de outro dono => core.ErrNotFound.
func (s *Store) UpdateAlertFireAt(ctx context.Context, userID, id int64, fireAt time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE alerts SET fire_at = $3, status = 'pending'
		   WHERE id = $1 AND user_id = $2`, id, userID, fireAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func scanAlert(row pgx.Row) (core.Alert, error) {
	var a core.Alert
	err := row.Scan(&a.ID, &a.Message, &a.NoteID, &a.FireAt, &a.Recurrence, &a.Status, &a.CreatedAt)
	return a, err
}

func scanAlerts(rows pgx.Rows) ([]core.Alert, error) {
	out := []core.Alert{} // nunca nil
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
