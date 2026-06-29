package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/Pedro-0101/gix-server/internal/ai"
	"github.com/Pedro-0101/gix-server/internal/core"
)

// buildAskPrompt monta o prompt de /ask: responde à pergunta usando APENAS as
// anotações fornecidas.
func buildAskPrompt(query string, results []core.SearchResult, language string) []ai.Message {
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] %s\n%s\n---\n", i+1, r.Title, r.Content)
	}
	system := fmt.Sprintf(`Você responde à pergunta do usuário usando APENAS as anotações fornecidas abaixo.
Resuma de forma direta em Markdown. Não invente informação que não esteja nas notas.
Se as notas não responderem à pergunta, diga isso claramente.
Idioma da resposta: %s.`, language)
	user := fmt.Sprintf("Pergunta:\n%s\n\nAnotações:\n%s", query, b.String())
	return []ai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// buildNoteSummaryPrompt monta o prompt de /resumir: condensa a nota em Markdown
// sem inventar nem adicionar informação.
func buildNoteSummaryPrompt(note core.Note, language string) []ai.Message {
	system := fmt.Sprintf(`Você resume uma anotação do usuário de forma concisa em Markdown.
Preserve os pontos principais; não invente nem adicione informação que não esteja na nota.
Mantenha o resultado bem mais curto que o original, em estrutura clara (parágrafos curtos, listas ou tarefas "- [ ]" quando fizer sentido).
Idioma da resposta: %s. Responda APENAS com o resumo, sem preâmbulo nem cercas.`, language)
	user := fmt.Sprintf("%s\n\n%s", note.Title, note.Content)
	return []ai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// buildNoteTidyPrompt monta o prompt de /tidy: reorganiza a nota preservando
// TODA a informação (não resume), melhorando estrutura e formatação Markdown.
func buildNoteTidyPrompt(note core.Note, language string) []ai.Message {
	system := fmt.Sprintf(`Você reorganiza uma anotação do usuário, melhorando a estrutura e a formatação SEM resumir.
Preserve TODA a informação e os fatos — não invente, não remova e não encurte o conteúdo.
Agrupe pontos relacionados, use títulos/seções, listas e tarefas "- [ ]" quando fizer sentido, corrija a formatação Markdown e ordene de forma lógica.
Idioma da resposta: %s. Responda APENAS com a nota reorganizada em Markdown, sem preâmbulo nem cercas.`, language)
	user := fmt.Sprintf("%s\n\n%s", note.Title, note.Content)
	return []ai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// buildCapturePrompt monta o prompt para formatar uma nota a partir de texto
// livre. Inclui as notas candidatas para attach routing.
func buildCapturePrompt(text string, now time.Time, cands []core.Note, language string) []ai.Message {
	stamp, zoneName, offsetH := localTimeHeader(now)
	system := fmt.Sprintf(`Você organiza anotações rápidas do usuário em uma nota atômica e bem formatada.
A data e hora atuais são: %s. Fuso: %s (UTC%+d).
Resolva qualquer data relativa ("amanhã", "sexta") para uma data absoluta no texto.
Formate "content" como Markdown bem estruturado (parágrafo, lista, tarefa "- [ ]", ou pequena seção) — preserve a informação do usuário, sem inventar nem remover.
Gere um "title" curto e específico (3 a 6 palavras, sem marcadores Markdown nem ponto final) que nomeie o assunto concreto da nota — evite títulos genéricos como "Nota", "Lembrete" ou "Ideia". Gere de 1 a 5 "tags" temáticas, minúsculas, sem "#".
Se o usuário pedir explicitamente para criar um alerta ou lembrete, extraia essa instrução para o campo "alert" — o conteúdo da nota deve conter apenas o restante do texto. Se não houver instrução explícita mas a nota descrever um lembrete com horário/data concretos, também inclua "alert". Caso contrário use "alert": null.%s
Responda APENAS com JSON, sem cercas, no idioma %s:
{"title":"<título curto>","content":"<Markdown da nota>","tags":["tag1","tag2"],"attach_to":null,"alert":null ou {"message":"<lembrete curto>","fireAt":"<ISO 8601 com offset>","recurrence":null ou {"freq":"daily|weekly|monthly|yearly","interval":1}}}`,
		stamp, zoneName, offsetH, candidatesBlock(cands), language)
	return []ai.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: text},
	}
}

// candidatesBlock renderiza as notas candidatas para o prompt de captura, com
// a regra de quando usar attach_to. Vazio quando não há candidatos.
func candidatesBlock(cands []core.Note) string {
	if len(cands) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nAnotações já existentes possivelmente relacionadas:\n")
	for _, n := range cands {
		fmt.Fprintf(&b, "- id %d — %s: %s\n", n.ID, n.Title, snippet(n.Content))
	}
	b.WriteString(`Se este novo texto for claramente a MESMA anotação/assunto de UMA delas (complementando-a, não apenas um tema parecido), coloque o id dela em "attach_to" para anexar. Na dúvida, ou se for algo novo, use "attach_to": null.`)
	return b.String()
}

// buildNoteSplitPrompt monta o prompt para dividir uma nota longa em várias.
func buildNoteSplitPrompt(title, content, language string) []ai.Message {
	system := fmt.Sprintf(`Você divide uma anotação longa em VÁRIAS anotações menores, agrupadas por tema/assunto.
Preserve TODA a informação e os fatos — não invente, não remova e não resuma. Cada informação deve aparecer em exatamente uma das notas.
Para cada nota gere um "title" curto e específico (3 a 6 palavras, sem marcadores Markdown) e de 1 a 5 "tags" temáticas, minúsculas, sem "#". Formate "content" como Markdown bem estruturado.
Crie de 2 a 6 notas. Idioma da resposta: %s. Responda APENAS com JSON, sem cercas:
{"notes":[{"title":"<título>","content":"<Markdown>","tags":["tag1"]}]}`, language)
	user := fmt.Sprintf("%s\n\n%s", title, content)
	return []ai.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}
