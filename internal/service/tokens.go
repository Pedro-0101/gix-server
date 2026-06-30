package service

import (
	"github.com/Pedro-0101/gix-server/internal/ai"
)

// defaultChatMaxTokens é o limite de tokens de contexto quando o usuário não
// configura um valor em chat_max_tokens (0 = usa este default).
const defaultChatMaxTokens = 96000

// estimateTokens estima o número de tokens de uma string. Sem tokenizer
// externo, usa a heurística de ~4 caracteres por token, comum para inglês e
// português.
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 {
		return 1
	}
	return n
}

// estimateMessagesTokens soma os tokens estimados de todas as mensagens.
func estimateMessagesTokens(msgs []ai.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m.Content)
	}
	return total
}

// trimHistory remove os pares user/assistant mais antigos do histórico até que
// o total estimado de tokens caiba no orçamento. Mantém os pares intactos —
// nunca quebra uma conversa no meio de um par.
func trimHistory(history []ai.Message, budgetTokens int) []ai.Message {
	if budgetTokens <= 0 || len(history) == 0 {
		return history
	}

	// Pré-computa tokens de cada mensagem para evitar re-escaneamento.
	tokens := make([]int, len(history))
	total := 0
	for i, m := range history {
		t := estimateTokens(m.Content)
		tokens[i] = t
		total += t
	}

	// Remove pares do início (mais antigos) enquanto estoura o orçamento.
	for total > budgetTokens && len(history) >= 2 {
		total -= tokens[0] + tokens[1]
		tokens = tokens[2:]
		history = history[2:]
	}

	// Se sobrou uma mensagem ímpar e ainda estoura, descarta.
	if len(history) == 1 && total > budgetTokens {
		history = nil
	}

	return history
}

// resolveMaxTokens devolve o limite efetivo de tokens de contexto: o valor
// configurado pelo usuário ou o default do servidor.
func resolveMaxTokens(userValue int) int {
	if userValue > 0 {
		return userValue
	}
	return defaultChatMaxTokens
}
