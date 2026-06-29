// Package recur é a especificação fechada de recorrência de alertas, portada
// 1:1 do gix (internal/app/recurrence.go). Lógica pura, sem dependências: o
// scheduler usa NextFireAt p/ reagendar alertas repetidos, e a fase 2 (IA) vai
// usar Marshal p/ serializar a regra que o modelo emite.
package recur

import (
	"encoding/json"
	"time"
)

// Rule é o spec mínimo e fechado de repetição. weekday/time são informativos
// (display + contrato com a IA); NextFireAt só usa freq+interval, porque o
// primeiro fire_at é um timestamp absoluto que já fixa a hora de parede e o dia.
type Rule struct {
	Freq     string `json:"freq"`              // daily|weekly|monthly|yearly
	Interval int    `json:"interval"`          // a cada N períodos
	Weekday  string `json:"weekday,omitempty"` // mon..sun, só weekly
	Time     string `json:"time,omitempty"`    // "09:00"
}

// maxSteps limita o catch-up p/ um alerta muito velho nunca girar pra sempre
// (ex.: um alerta diário intocado por anos).
const maxSteps = 4000

// Parse decodifica uma string de recorrência guardada. ok=false p/ a string
// vazia (one-shot) ou JSON inválido / sem freq.
func Parse(s string) (Rule, bool) {
	if s == "" {
		return Rule{}, false
	}
	var r Rule
	if err := json.Unmarshal([]byte(s), &r); err != nil || r.Freq == "" {
		return Rule{}, false
	}
	return r, true
}

// Marshal serializa uma regra, ou "" quando não há (one-shot).
func Marshal(r *Rule) string {
	if r == nil || r.Freq == "" {
		return ""
	}
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(b)
}

// NextFireAt devolve a próxima ocorrência estritamente depois de now, avançando
// a partir do disparo anterior `last` por períodos inteiros de freq×interval. A
// aritmética acontece na location de `last`, então hora de parede e dia da
// semana ficam estáveis no DST. Um alerta repetido velho avança uma vez além de
// now (sem backlog de spam).
func NextFireAt(rule Rule, last, now time.Time) time.Time {
	next := last
	for i := 0; i < maxSteps; i++ {
		next = advance(rule, next)
		if next.After(now) {
			return next
		}
	}
	return next
}

// advance dá um passo de um período freq×interval (>=1 período).
func advance(rule Rule, t time.Time) time.Time {
	n := rule.Interval
	if n < 1 {
		n = 1
	}
	switch rule.Freq {
	case "weekly":
		return t.AddDate(0, 0, 7*n)
	case "monthly":
		return t.AddDate(0, n, 0)
	case "yearly":
		return t.AddDate(n, 0, 0)
	default: // daily
		return t.AddDate(0, 0, n)
	}
}
