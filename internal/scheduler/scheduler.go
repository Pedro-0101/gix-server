// Package scheduler é o disparo server-side de alertas. Uma goroutine varre os
// alertas pendentes vencidos (de todos os usuários), enfileira a entrega no
// outbox e empurra pelo Notifier; depois avança a recorrência no fuso do dono
// ou marca o one-shot como done. Funciona com o desktop fechado: o disparo e o
// avanço acontecem no servidor; a entrega ao vivo é só uma das pontas.
//
// O Notifier é o ponto de extensão de transporte (hoje SSE pro desktop; amanhã
// WhatsApp/Telegram) — o scheduler não sabe qual canal é, igual ao core.
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/Pedro-0101/gix-server/internal/recur"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// pollInterval é a cadência da varredura por alertas vencidos.
const pollInterval = 30 * time.Second

// Notifier entrega um disparo ao usuário. A implementação (push hub SSE) decide
// o transporte; devolver sem erro não garante entrega (cliente pode estar
// offline) — por isso o outbox persiste tudo.
type Notifier interface {
	Deliver(userID int64, d store.Delivery)
}

// Scheduler liga o store ao Notifier.
type Scheduler struct {
	store    *store.Store
	notifier Notifier
}

func New(s *store.Store, n Notifier) *Scheduler {
	return &Scheduler{store: s, notifier: n}
}

// Run roda o loop: um tick imediato no boot (pega alertas que venceram com o
// servidor desligado), depois a cada pollInterval até ctx encerrar.
func (s *Scheduler) Run(ctx context.Context) {
	s.tick(ctx, time.Now())
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx, time.Now())
		}
	}
}

// tick dispara cada alerta vencido uma vez: grava a entrega no outbox, empurra
// pelo Notifier e então reagenda os recorrentes p/ a próxima ocorrência futura
// (no fuso do dono) ou marca os one-shots como done.
func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	due, err := s.store.DueAlerts(ctx, now)
	if err != nil {
		log.Printf("scheduler: due alerts: %v", err)
		return
	}
	for _, a := range due {
		d, err := s.store.CreateDelivery(ctx, a.UserID, a.ID, a.Message, a.NoteID, a.FireAt)
		if err != nil {
			log.Printf("scheduler: enfileirar entrega do alerta %d: %v", a.ID, err)
			continue // sem outbox não dá p/ garantir entrega; tenta no próximo tick
		}
		s.notifier.Deliver(a.UserID, d)

		if rule, ok := recur.Parse(a.Recurrence); ok {
			loc := location(a.Timezone)
			next := recur.NextFireAt(rule, a.FireAt.In(loc), now.In(loc))
			if err := s.store.UpdateAlertFireAt(ctx, a.UserID, a.ID, next.UTC()); err != nil {
				log.Printf("scheduler: reagendar alerta %d: %v", a.ID, err)
			}
			continue
		}
		if err := s.store.SetAlertStatus(ctx, a.UserID, a.ID, "done"); err != nil {
			log.Printf("scheduler: marcar done alerta %d: %v", a.ID, err)
		}
	}
}

// location resolve o fuso do usuário; fuso inválido cai p/ UTC (nunca trava o
// disparo). Depende do tzdata embutido (ver _ "time/tzdata" no main).
func location(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}
