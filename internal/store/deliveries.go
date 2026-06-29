package store

import (
	"context"
	"time"
)

// Delivery é uma entrega de alerta no outbox: um disparo do scheduler que precisa
// chegar ao usuário. delivered_at marca quando o push (SSE) confirmou a escrita;
// enquanto NULL, fica pendente e é reenviada no próximo connect.
type Delivery struct {
	ID      int64     `json:"deliveryId"`
	AlertID *int64    `json:"alertId"` // nil se o alerta foi apagado depois
	Message string    `json:"message"`
	NoteID  *int64    `json:"noteId"`
	FireAt  time.Time `json:"fireAt"`
}

// CreateDelivery grava no outbox o disparo de um alerta (ainda não entregue).
func (s *Store) CreateDelivery(ctx context.Context, userID, alertID int64, message string, noteID *int64, fireAt time.Time) (Delivery, error) {
	d := Delivery{AlertID: &alertID, Message: message, NoteID: noteID, FireAt: fireAt}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO alert_deliveries (user_id, alert_id, message, note_id, fire_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, alertID, message, noteID, fireAt,
	).Scan(&d.ID)
	return d, err
}

// UndeliveredDeliveries retorna as entregas pendentes do usuário, mais antigas
// primeiro (ordem de disparo). Usado no flush ao conectar no SSE. Nunca nil.
func (s *Store) UndeliveredDeliveries(ctx context.Context, userID int64) ([]Delivery, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, alert_id, message, note_id, fire_at
		   FROM alert_deliveries
		  WHERE user_id = $1 AND delivered_at IS NULL
		  ORDER BY fire_at ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.AlertID, &d.Message, &d.NoteID, &d.FireAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkDelivered marca uma entrega como entregue (idempotente: só toca as ainda
// pendentes, então flush no connect e push ao vivo não brigam).
func (s *Store) MarkDelivered(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE alert_deliveries SET delivered_at = now()
		   WHERE id = $1 AND delivered_at IS NULL`, id)
	return err
}
