package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Pedro-0101/gix-server/internal/core"
)

// alertCols é a ordem canônica de leitura de um alerta.
const alertCols = "id, message, note_id, fire_at, recurrence, status, created_at, COALESCE(google_calendar_event_id,'')"

// ListAlerts retorna os alertas do usuário, mais cedo primeiro. Nunca nil.
func (s *Store) ListAlerts(ctx context.Context, userID int64, p Pagination) ([]core.Alert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+alertCols+` FROM alerts
		   WHERE user_id = $1 ORDER BY fire_at ASC, id ASC
		   LIMIT $2 OFFSET $3`, userID, p.Limit, p.Offset)
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

// SetGCalEventID vincula um alerta a um evento do Google Calendar.
func (s *Store) SetGCalEventID(ctx context.Context, id int64, eventID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE alerts SET google_calendar_event_id = $2 WHERE id = $1`, id, eventID)
	return err
}

// DueAlert é um alerta pendente vencido, com o dono e o fuso resolvidos — o que
// o scheduler precisa p/ rotear o push e avançar a recorrência na parede certa.
// core.Alert não carrega user_id/timezone (é escopado por fora); o scheduler é
// cross-user, então estes campos vêm junto aqui.
type DueAlert struct {
	core.Alert
	UserID   int64
	Timezone string
}

// DueAlerts retorna, de TODOS os usuários, os alertas pendentes cujo fire_at já
// passou — a varredura do scheduler. O fuso vem do user_prefs (default UTC).
func (s *Store) DueAlerts(ctx context.Context, now time.Time) ([]DueAlert, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.message, a.note_id, a.fire_at, a.recurrence, a.status, a.created_at,
		        COALESCE(a.google_calendar_event_id,''), a.user_id, COALESCE(p.timezone, 'UTC')
		   FROM alerts a
		   LEFT JOIN user_prefs p ON p.user_id = a.user_id
		  WHERE a.status = 'pending' AND a.fire_at <= $1
		  ORDER BY a.fire_at ASC, a.id ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DueAlert{}
	for rows.Next() {
		var d DueAlert
		if err := rows.Scan(&d.ID, &d.Message, &d.NoteID, &d.FireAt, &d.Recurrence,
			&d.Status, &d.CreatedAt, &d.GoogleCalendarEventID, &d.UserID, &d.Timezone); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanAlert(row pgx.Row) (core.Alert, error) {
	var a core.Alert
	err := row.Scan(&a.ID, &a.Message, &a.NoteID, &a.FireAt, &a.Recurrence, &a.Status, &a.CreatedAt, &a.GoogleCalendarEventID)
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
