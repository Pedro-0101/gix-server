// Package core é o cérebro agnóstico de canal do gix-server: toda a lógica de
// negócio (captura, busca, IA, alertas) vive aqui, exposta como "intents" de
// entrada/saída em dados simples. Nenhum tipo deste pacote sabe o que é desktop,
// WhatsApp, Telegram ou web — só os adapters de canal (camada de transporte)
// traduzem entre o canal e estas intents. É isso que garante a mesma lógica e a
// mesma resposta em todos os canais.
//
// As tags JSON definem o contrato de wire da API (camelCase). É o formato que
// todo canal consome; manter estável.
package core

import "time"

// Note é uma anotação do usuário.
type Note struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	CharLimit int       `json:"charLimit"` // 0 = herda o limite global do usuário
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Alert é um lembrete agendado. O disparo é server-side (ver scheduler).
type Alert struct {
	ID         int64     `json:"id"`
	Message    string    `json:"message"`
	NoteID     *int64    `json:"noteId"` // vínculo fraco com a nota de origem
	FireAt     time.Time `json:"fireAt"`
	Recurrence string    `json:"recurrence"` // "" = one-shot, senão regra JSON
	Status     string    `json:"status"`     // pending | done | cancelled
	CreatedAt  time.Time `json:"createdAt"`
}

// Conversation é uma sessão de chat.
type Conversation struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"createdAt"`
}

// Message é uma mensagem dentro de uma conversa.
type Message struct {
	ID      int64  `json:"id"`
	Role    string `json:"role"` // user | assistant | system
	Content string `json:"content"`
}

// Usage é o custo de uma chamada de IA, propagado de volta pro canal.
type Usage struct {
	Tokens int     `json:"tokens"`
	Cost   float64 `json:"cost"` // USD
}

// SearchResult é um acerto da busca híbrida (FTS + vetor, fundidos por RRF).
type SearchResult struct {
	NoteID  int64    `json:"noteId"`
	Title   string   `json:"title"`
	Snippet string   `json:"snippet"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Score   float64  `json:"score"`
}

// CaptureResult é o desfecho de uma captura (/note): o core decide criar nota
// nova, propor anexar a uma existente, ou sinalizar overflow. O canal confirma
// conforme suas capacidades (botão no desktop, resposta de texto num bot).
type CaptureResult struct {
	Status    string            `json:"status"` // created | attach_proposed | overflow_proposed | no_api_key | error
	NoteID    int64             `json:"noteId"`
	NoteTitle string            `json:"noteTitle"`
	Content   string            `json:"content"`
	Tags      []string          `json:"tags"`
	Message   string            `json:"message"`
	Usage     Usage             `json:"usage"`
	Alert     *AlertProposal    `json:"alert"`    // lembrete detectado no texto
	Attach    *AttachProposal   `json:"attach"`   // sugestão de anexar a uma nota existente
	Overflow  *OverflowProposal `json:"overflow"` // conteúdo estouraria o limite efetivo
}

// AskResult é a resposta de /ask: resumo de IA sobre as notas mais relevantes.
type AskResult struct {
	Status  string         `json:"status"` // ok | no_api_key | error
	Summary string         `json:"summary"`
	Sources []SearchResult `json:"sources"`
	Message string         `json:"message"`
	Usage   Usage          `json:"usage"`
}

// SummarizeResult / TidyResult: a IA reescreve o corpo da nota (resumir/organizar)
// e devolve o texto; quem aplica/desfaz é a camada acima.
type SummarizeResult struct {
	Status  string `json:"status"` // ok | no_api_key | empty | error
	Summary string `json:"summary"`
	Message string `json:"message"`
	Usage   Usage  `json:"usage"`
}

type TidyResult struct {
	Status  string `json:"status"` // ok | no_api_key | empty | error
	Content string `json:"content"`
	Message string `json:"message"`
	Usage   Usage  `json:"usage"`
}

// CreateAlertResult é o desfecho de criar um lembrete a partir de linguagem natural.
type CreateAlertResult struct {
	Status      string `json:"status"` // created | no_api_key | unparseable | past | error
	AlertID     int64  `json:"alertId"`
	Message     string `json:"message"`
	FireAtLocal string `json:"fireAtLocal"` // ISO 8601 no fuso do usuário
	Recurrence  string `json:"recurrence"`
	Usage       Usage  `json:"usage"`
}

// GraphData alimenta o visualizador estilo obsidian (notas ligadas por tags).
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

type GraphEdge struct {
	Source int64 `json:"source"`
	Target int64 `json:"target"`
}

// Propostas vindas da IA (tool-calls / roteamento). São dados simples; o canal
// as apresenta e confirma do jeito dele.
type AlertProposal struct {
	Message    string `json:"message"`
	FireAt     string `json:"fireAt"` // ISO 8601 com offset
	Recurrence string `json:"recurrence"`
}

type AttachProposal struct {
	TargetID    int64  `json:"targetId"`
	TargetTitle string `json:"targetTitle"`
}

type OverflowProposal struct {
	TargetID    int64  `json:"targetId"`
	TargetTitle string `json:"targetTitle"`
	Length      int    `json:"length"`
	Limit       int    `json:"limit"`
}
