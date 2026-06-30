package gcal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/recur"
)

func eventFromAlert(alert *core.Alert) *calendar.Event {
	start := &calendar.EventDateTime{DateTime: alert.FireAt.Format(time.RFC3339)}
	end := &calendar.EventDateTime{DateTime: alert.FireAt.Add(30 * time.Minute).Format(time.RFC3339)}

	event := &calendar.Event{
		Summary: alert.Message,
		Start:   start,
		End:     end,
	}

	if r, ok := recur.Parse(alert.Recurrence); ok {
		if rrule := recurToRRULE(r); rrule != "" {
			event.Recurrence = []string{rrule}
		}
	}

	if alert.NoteID != nil {
		noteRef := fmt.Sprintf("gix-note:%d", *alert.NoteID)
		event.Description = noteRef
	}

	return event
}

func (c *Client) CreateEvent(ctx context.Context, userID int64, alert *core.Alert) (string, error) {
	svc, err := c.calendarService(ctx, userID)
	if err != nil {
		return "", err
	}

	event := eventFromAlert(alert)
	created, err := svc.Events.Insert("primary", event).Do()
	if err != nil {
		return "", err
	}

	slog.Info("gcal: evento criado", "alertID", alert.ID, "eventID", created.Id)
	return created.Id, nil
}

func (c *Client) UpdateEvent(ctx context.Context, userID int64, eventID string, alert *core.Alert) error {
	svc, err := c.calendarService(ctx, userID)
	if err != nil {
		return err
	}

	event := eventFromAlert(alert)
	if _, err := svc.Events.Update("primary", eventID, event).Do(); err != nil {
		return err
	}

	slog.Info("gcal: evento atualizado", "alertID", alert.ID, "eventID", eventID)
	return nil
}

func (c *Client) DeleteEvent(ctx context.Context, userID int64, eventID string) error {
	svc, err := c.calendarService(ctx, userID)
	if err != nil {
		return err
	}

	if err := svc.Events.Delete("primary", eventID).Do(); err != nil {
		return err
	}

	slog.Info("gcal: evento removido", "eventID", eventID)
	return nil
}
