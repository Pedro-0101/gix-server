package service

import (
	"fmt"
	"time"

	"github.com/Pedro-0101/gix-server/internal/ai"
)

// buildAlertPrompt monta o prompt para a IA extrair data/hora de um texto em
// linguagem natural. Recebe o texto do usuário, contexto adicional, o agora no
// fuso do usuário e o idioma.
func buildAlertPrompt(text, ctxMsg string, now time.Time, lang string) []ai.Message {
	timeHeader := fmt.Sprintf(
		`A data/hora atual é %s (fuso %s).
Use este fuso como referência para interpretar datas e horários relativos.`,
		now.Format("2006-01-02 15:04:05 (MST -0700)"),
		now.Location().String(),
	)
	system := fmt.Sprintf(`Você extrai informações de lembrete de um texto em linguagem natural.

%s

O usuário escreveu em %s.

Responda APENAS com um JSON neste formato exato, sem cercas nem preâmbulo:
{
  "message": "texto do lembrete (extraído ou reformulado)",
  "fireAt": "2006-01-02T15:04:00-07:00 (ISO 8601 com offset, no fuso do usuário)",
  "recurrence": "" (vazio para one-shot) ou "{\"freq\":\"daily\",\"interval\":1}" (JSON com regra de recorrência)
}

Regras:
- Se o texto não contiver data/hora, use "fireAt" vazio.
- Se mencionar "todo dia", "diariamente" etc., recurrence = {"freq":"daily","interval":1}.
- Se mencionar "toda semana", "semanalmente" etc., recurrence = {"freq":"weekly","interval":1}.
- Se mencionar "todo mês", "mensalmente" etc., recurrence = {"freq":"monthly","interval":1}.
- Se mencionar "todo ano", "anualmente" etc., recurrence = {"freq":"yearly","interval":1}.
- Intervalos diferentes de 1: {"freq":"daily","interval":2} (a cada 2 dias).
- Dias da semana: {"freq":"weekly","byday":"mon,wed,fri"}.
- Se fireAt estiver no passado para um alerta não-recorrente, sinalize colocando fireAt vazio.
- Para recorrentes, o primeiro fireAt pode estar no passado (o scheduler avança).`,
		timeHeader, lang)

	user := text
	if ctxMsg != "" {
		user = fmt.Sprintf("Contexto: %s\n\nTexto: %s", ctxMsg, text)
	}
	return []ai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}
