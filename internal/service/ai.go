package service

import (
	"context"
	"strings"
	"time"

	"github.com/Pedro-0101/gix-server/internal/ai"
	"github.com/Pedro-0101/gix-server/internal/config"
	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// resolveKey retorna a chave OpenRouter efetiva para o usuário: primeiro
// tenta a chave configurada nos user_prefs (openrouter_key); se não houver,
// cai para a chave do servidor (OPENROUTER_API_KEY). Retorna "" se nenhuma
// estiver disponível.
func (a AI) resolveKey(ctx context.Context, s *store.Store, userID int64) string {
	if s != nil {
		key, err := s.GetUserOpenRouterKey(ctx, userID)
		if err == nil && key != "" {
			return key
		}
	}
	if a.Client != nil {
		return a.Client.Key()
	}
	return ""
}

// hasKey informa se há chave de IA configurada no servidor (não por usuário).
// Intents que precisam de chave por usuário usam resolveKey.
func (a AI) hasKey() bool { return a.Client != nil && a.Client.HasKey() }

// lang returns the user language or the default "Português do Brasil".
func lang(l string) string {
	if l == "" {
		return "Português do Brasil"
	}
	return l
}

// complete faz uma chamada não-streaming e devolve o texto (sem cercas) e o
// Usage já convertido para o contrato do core (tokens + custo no modelo).
// apiKey é a chave do usuário (vinda dos prefs); se vazia, usa a chave do servidor.
func (a AI) complete(ctx context.Context, apiKey string, msgs []ai.Message) (string, core.Usage, error) {
	raw, usage, err := a.Client.Complete(ctx, apiKey, a.Model, msgs)
	if err != nil {
		return "", core.Usage{}, err
	}
	return strings.TrimSpace(stripFences(raw)), a.usage(usage), nil
}

// usage converte o Usage da OpenRouter no Usage do core, calculando o custo a
// partir da tabela de preços do modelo (modelo desconhecido => custo 0).
func (a AI) usage(u *ai.Usage) core.Usage {
	if u == nil {
		return core.Usage{}
	}
	cost := 0.0
	if p, ok := config.ModelPrices[a.Model]; ok {
		cost = p.CalculateCost(u.PromptTokens, u.CompletionTokens)
	}
	return core.Usage{Tokens: u.TotalTokens, Cost: cost}
}

// localTimeHeader formata o momento atual para prompts de IA: um carimbo
// legível, o nome do fuso e o offset em horas. Usado por capture, chat e alertas.
func localTimeHeader(now time.Time) (stamp, zone string, offsetHours int) {
	zoneName, offsetSec := now.Zone()
	return now.Format("2006-01-02 15:04:05 (Monday)"), zoneName, offsetSec / 3600
}

// extractTitle extrai um título curto do conteúdo de uma nota — primeira linha
// se houver, ou os primeiros 60 caracteres.
func extractTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "Nota"
	}
	if idx := strings.IndexByte(content, '\n'); idx != -1 {
		title := strings.TrimSpace(content[:idx])
		if title != "" {
			return title
		}
	}
	runes := []rune(content)
	if len(runes) > 60 {
		return string(runes[:60]) + "…"
	}
	return content
}

// futureOrRecurring diz se um alerta parseado vale a pena propor: fire time
// válido que seja recorrente ou ainda no futuro.
func futureOrRecurring(fireAtISO, recurrence string, now time.Time) bool {
	fireAt, err := time.Parse(time.RFC3339, strings.TrimSpace(fireAtISO))
	if err != nil {
		return false
	}
	return recurrence != "" || fireAt.Add(gracePeriod).After(now)
}

// stripFences remove uma cerca ```...``` ao redor de uma resposta de IA, quando
// o modelo embrulha o resultado apesar de instruído a não fazê-lo.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i != -1 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
