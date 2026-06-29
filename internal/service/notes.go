// Package service implementa as intents do core (internal/core) sobre o store
// Postgres. Fica separado de core pra evitar ciclo de import: store depende de
// core (tipos), e service depende de core + store.
//
// Fase 2: busca híbrida (Find/Ask), IA (Summarize/Tidy/Capture) e alertas por
// linguagem natural funcionam. Chat e Capture com roteamento ainda pendentes.
package service

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Pedro-0101/gix-server/internal/core"
	"github.com/Pedro-0101/gix-server/internal/store"
)

// Tunables da busca híbrida e do roteamento de captura.
const (
	candidateLimit = 30  // candidatos por fonte (FTS / vetor) antes da fusão
	rrfK           = 60  // constante do Reciprocal Rank Fusion
	snippetRunes   = 180 // máximo de runas no snippet de prévia
	askTopK        = 6   // notas enviadas à IA no /ask

	maxCandidates = 5   // candidatos oferecidos ao modelo no attach routing
	attachMinSim  = 0.82 // similaridade mínima para ser candidato a attach
)

// embedNote gera e persiste o embedding para o texto da nota, se o embedder
// estiver disponível. Falhas de embedding não quebram a operação principal.
func (n *Notes) embedNote(ctx context.Context, userID, noteID int64, title, content string) {
	if n.ai.Embedder == nil {
		return
	}
	text := title
	if content != "" {
		text += "\n\n" + content
	}
	emb, err := n.ai.Embedder.EmbedPassage(text)
	if err != nil {
		return
	}
	n.store.UpdateNoteEmbedding(ctx, userID, noteID, emb) //nolint:errcheck
}

// Notes implementa core.Notes.
type Notes struct {
	store *store.Store
	ai    AI
}

func NewNotes(s *store.Store, aiDeps AI) *Notes { return &Notes{store: s, ai: aiDeps} }

var _ core.Notes = (*Notes)(nil)

// loadLang carrega o idioma do usuário das preferências.
func (n *Notes) loadLang(ctx context.Context, userID int64) string {
	prefs, err := n.store.GetUserPrefs(ctx, userID)
	if err != nil {
		return ""
	}
	return prefs.Language
}

// loadPrefs carrega as preferências completas do usuário.
func (n *Notes) loadPrefs(ctx context.Context, userID int64) store.UserPrefs {
	prefs, err := n.store.GetUserPrefs(ctx, userID)
	if err != nil {
		return store.UserPrefs{}
	}
	return prefs
}

// --- CRUD (sem IA) ---------------------------------------------------------

func (n *Notes) List(ctx context.Context, userID int64, opts core.ListOptions) ([]core.Note, error) {
	return n.store.ListNotes(ctx, userID, store.Pagination{Limit: opts.Limit, Offset: opts.Offset})
}

func (n *Notes) Get(ctx context.Context, userID, id int64) (core.Note, error) {
	return n.store.GetNote(ctx, userID, id)
}

func (n *Notes) Update(ctx context.Context, userID, id int64, title, content string, tags []string) (core.Note, error) {
	note, err := n.store.UpdateNote(ctx, userID, id, title, content, tags)
	if err != nil {
		return core.Note{}, err
	}
	n.embedNote(ctx, userID, note.ID, note.Title, note.Content)
	return note, nil
}

func (n *Notes) Delete(ctx context.Context, userID, id int64) error {
	return n.store.DeleteNote(ctx, userID, id)
}

func (n *Notes) SetCharLimit(ctx context.Context, userID, id int64, limit int) error {
	return n.store.SetCharLimit(ctx, userID, id, limit)
}

// CreateFromProposal grava uma nota a partir de campos já prontos (criação
// manual ou confirmação de uma proposta de captura) — sem chamar IA.
func (n *Notes) CreateFromProposal(ctx context.Context, userID int64, title, content string, tags []string) (core.CaptureResult, error) {
	prefs := n.loadPrefs(ctx, userID)
	note, err := n.store.CreateNote(ctx, userID, title, content, tags, prefs.CharLimit)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	n.embedNote(ctx, userID, note.ID, note.Title, note.Content)
	if prefs.CharLimit > 0 && utf8.RuneCountInString(note.Content) > prefs.CharLimit {
		return core.CaptureResult{
			Status: "overflow_proposed", NoteID: note.ID, NoteTitle: note.Title,
			Content: note.Content, Tags: note.Tags,
			Overflow: &core.OverflowProposal{
				TargetID: note.ID, TargetTitle: note.Title,
				Length: utf8.RuneCountInString(note.Content), Limit: prefs.CharLimit,
			},
		}, nil
	}
	return core.CaptureResult{
		Status:    "created",
		NoteID:    note.ID,
		NoteTitle: note.Title,
		Content:   note.Content,
		Tags:      note.Tags,
	}, nil
}

// AppendTo anexa conteúdo a uma nota existente e faz união das tags (sem IA;
// re-embedding entra na fase 2). Se exceder o limite, propõe overflow.
func (n *Notes) AppendTo(ctx context.Context, userID, targetID int64, content string, tags []string) (core.CaptureResult, error) {
	note, err := n.store.GetNote(ctx, userID, targetID)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	merged := note.Content
	if content != "" {
		merged += "\n\n" + content
	}
	prefs := n.loadPrefs(ctx, userID)
	limit := note.CharLimit
	if limit <= 0 {
		limit = prefs.CharLimit
	}
	if prop := n.overflowProposal(note, merged, limit); prop != nil {
		return core.CaptureResult{
			Status: "overflow_proposed", NoteTitle: note.Title,
			Content: merged, Tags: unionTags(note.Tags, tags),
			Overflow: prop,
		}, nil
	}
	updated, err := n.store.UpdateNote(ctx, userID, targetID, note.Title, merged, unionTags(note.Tags, tags))
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	n.embedNote(ctx, userID, updated.ID, updated.Title, updated.Content)
	return core.CaptureResult{
		Status:    "created",
		NoteID:    updated.ID,
		NoteTitle: updated.Title,
		Content:   updated.Content,
		Tags:      updated.Tags,
	}, nil
}

// Graph monta nós + arestas: duas notas se ligam se compartilham >=1 tag.
func (n *Notes) Graph(ctx context.Context, userID int64) (core.GraphData, error) {
	notes, err := n.store.GraphNotes(ctx, userID)
	if err != nil {
		return core.GraphData{}, err
	}
	g := core.GraphData{Nodes: []core.GraphNode{}, Edges: []core.GraphEdge{}} // nunca nil
	for _, nt := range notes {
		g.Nodes = append(g.Nodes, core.GraphNode{ID: nt.ID, Title: nt.Title, Tags: nt.Tags})
	}
	for i := 0; i < len(notes); i++ {
		for j := i + 1; j < len(notes); j++ {
			if shareTag(notes[i].Tags, notes[j].Tags) {
				g.Edges = append(g.Edges, core.GraphEdge{Source: notes[i].ID, Target: notes[j].ID})
			}
		}
	}
	return g, nil
}

// BackfillEmbeddings gera embeddings para notas que ainda não têm, em lotes.
// Retorna o total de notas embedadas. Não falha na primeira nota com erro —
// loga e continua.
func (n *Notes) BackfillEmbeddings(ctx context.Context, batchSize int) (int, error) {
	if n.ai.Embedder == nil {
		return 0, nil
	}
	total := 0
	for {
		notes, err := n.store.NotesWithoutEmbedding(ctx, batchSize)
		if err != nil {
			return total, err
		}
		if len(notes) == 0 {
			return total, nil
		}
		for _, ns := range notes {
			text := ns.Title
			if ns.Content != "" {
				text += "\n\n" + ns.Content
			}
			emb, err := n.ai.Embedder.EmbedPassage(text)
			if err != nil {
				continue
			}
			if err := n.store.UpdateNoteEmbedding(ctx, ns.UserID, ns.ID, emb); err != nil {
				continue
			}
			total++
		}
	}
}

// --- busca híbrida (Find) e IA (Ask) ---------------------------------------

// Find busca notas por busca híbrida (FTS + similaridade vetorial, fundidos por
// RRF). Sem IA, sem custo.
func (n *Notes) Find(ctx context.Context, userID int64, query string) ([]core.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	rankings := make([][]int64, 0, 2)

	// FTS — sempre disponível
	ftsHits, err := n.store.SearchFTS(ctx, userID, query, candidateLimit)
	if err != nil {
		return nil, err
	}
	if len(ftsHits) > 0 {
		ids := make([]int64, len(ftsHits))
		for i, h := range ftsHits {
			ids[i] = h.NoteID
		}
		rankings = append(rankings, ids)
	}

	// Vetorial — só se o embedder estiver pronto
	if n.ai.Embedder != nil {
		vecIDs, err := n.vectorSearch(ctx, userID, query)
		if err != nil {
			return nil, err
		}
		if len(vecIDs) > 0 {
			rankings = append(rankings, vecIDs)
		}
	}

	if len(rankings) == 0 {
		return nil, nil
	}

	fused := rrf(rankings, rrfK)
	if len(fused) == 0 {
		return nil, nil
	}

	ids := make([]int64, len(fused))
	for i, f := range fused {
		ids[i] = f.id
	}

	notes, err := n.store.NotesByIDs(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]core.Note, len(notes))
	for _, note := range notes {
		byID[note.ID] = note
	}

	results := make([]core.SearchResult, 0, len(fused))
	for _, f := range fused {
		note, ok := byID[f.id]
		if !ok {
			continue
		}
		results = append(results, core.SearchResult{
			NoteID:  note.ID,
			Title:   note.Title,
			Snippet: snippet(note.Content),
			Content: note.Content,
			Tags:    note.Tags,
			Score:   f.score,
		})
	}
	return results, nil
}

// vectorSearch embeda a consulta e busca por similaridade de cosseno no
// pgvector, retornando até candidateLimit IDs ordenados por relevância.
func (n *Notes) vectorSearch(ctx context.Context, userID int64, query string) ([]int64, error) {
	if n.ai.Embedder == nil {
		return nil, nil
	}
	qEmb, err := n.ai.Embedder.EmbedQuery(query)
	if err != nil {
		return nil, nil // degrada para FTS-only
	}
	hits, err := n.store.SearchVector(ctx, userID, qEmb, candidateLimit)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(hits))
	for i, h := range hits {
		ids[i] = h.NoteID
	}
	return ids, nil
}

// rrf funde várias listas ranqueadas com Reciprocal Rank Fusion: o score de um
// item é a soma sobre as listas de 1/(k + rank), onde rank é 0-based. Maior é
// melhor.
func rrf(rankings [][]int64, k float64) []fusedItem {
	scores := map[int64]float64{}
	for _, ranking := range rankings {
		for rank, id := range ranking {
			scores[id] += 1 / (k + float64(rank))
		}
	}
	out := make([]fusedItem, 0, len(scores))
	for id, sc := range scores {
		out = append(out, fusedItem{id: id, score: sc})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].id > out[j].id
	})
	return out
}

type fusedItem struct {
	id    int64
	score float64
}

// Ask busca as notas mais relevantes (Find) e pede à IA um resumo em Markdown
// que responda à pergunta do usuário. Retorna o resumo mais as fontes.
func (n *Notes) Ask(ctx context.Context, userID int64, query string) (core.AskResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return core.AskResult{Status: "error", Message: "empty"}, nil
	}

	results, err := n.Find(ctx, userID, query)
	if err != nil {
		return core.AskResult{}, err
	}
	if len(results) == 0 {
		return core.AskResult{Status: "empty"}, nil
	}
	if len(results) > askTopK {
		results = results[:askTopK]
	}

	if !n.ai.hasKey() {
		return core.AskResult{Status: "no_api_key", Sources: results}, nil
	}

	language := n.loadLang(ctx, userID)
	summary, usage, err := n.ai.complete(ctx, buildAskPrompt(query, results, lang(language)))
	if err != nil {
		return core.AskResult{Status: "error", Message: err.Error(), Sources: results}, nil
	}

	return core.AskResult{
		Status:  "ok",
		Summary: summary,
		Sources: results,
		Usage:   usage,
	}, nil
}

// snippet retorna uma prévia curta (uma linha) do conteúdo da nota.
func snippet(content string) string {
	flat := strings.Join(strings.Fields(strings.ReplaceAll(content, "\n", " ")), " ")
	r := []rune(flat)
	if len(r) > snippetRunes {
		return strings.TrimSpace(string(r[:snippetRunes])) + "…"
	}
	return flat
}

// --- captura com IA (Notes.Capture + ResolveOverflow) -----------------------

// captureDecision é o JSON que a IA devolve ao organizar uma captura.
type captureDecision struct {
	Title    string          `json:"title"`
	Content  string          `json:"content"`
	Tags     []string        `json:"tags"`
	AttachTo *int64          `json:"attach_to"`
	Alert    json.RawMessage `json:"alert"` // alertDecision ou null
}

// captureAlertDec é o sub-objeto "alert" dentro do captureDecision, com
// recurrence como RawMessage porque o modelo retorna objeto ou null.
type captureAlertDec struct {
	Message    string          `json:"message"`
	FireAt     string          `json:"fireAt"`
	Recurrence json.RawMessage `json:"recurrence"`
}

// Capture recebe texto livre do usuário, chama a IA para estruturar em nota
// (título + conteúdo + tags), e decide se cria uma nota nova ou propõe anexar
// a uma existente (via candidateNotes). Retorna também propostas de alerta
// detectadas no texto.
func (n *Notes) Capture(ctx context.Context, userID int64, text string) (core.CaptureResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return core.CaptureResult{Status: "error", Message: "empty"}, nil
	}
	if !n.ai.hasKey() {
		return core.CaptureResult{Status: "no_api_key"}, nil
	}

	cands := n.candidateNotes(ctx, userID, text)
	language := n.loadLang(ctx, userID)

	raw, usage, err := n.ai.complete(ctx, buildCapturePrompt(text, time.Now(), cands, lang(language)))
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, nil
	}

	dec, err := parseCaptureJSON(raw)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, nil
	}

	content := strings.TrimSpace(dec.Content)
	if content == "" {
		content = text
	}
	title := strings.TrimSpace(dec.Title)
	if title == "" {
		title = extractTitle(content)
	}
	tags := normalizeTags(dec.Tags)

	alertProposal := n.buildAlertProposal(dec.Alert, title)

	// Routing: se o modelo escolheu uma nota candidata válida, propõe attach
	if target, ok := validAttach(dec.AttachTo, cands); ok {
		return core.CaptureResult{
			Status: "attach_proposed", NoteTitle: title, Content: content, Tags: tags,
			Usage: usage, Alert: alertProposal,
			Attach: &core.AttachProposal{TargetID: target.ID, TargetTitle: target.Title},
		}, nil
	}

	note, err := n.store.CreateNote(ctx, userID, title, content, tags, 0)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	n.embedNote(ctx, userID, note.ID, note.Title, note.Content)

	return core.CaptureResult{
		Status: "created", NoteID: note.ID, NoteTitle: note.Title,
		Content: note.Content, Tags: note.Tags, Usage: usage, Alert: alertProposal,
	}, nil
}

// candidateNotes busca as notas mais similares semanticamente ao texto, acima
// do limiar attachMinSim, para o roteador de captura decidir se anexa. Usa
// apenas busca vetorial (sem FTS). Devolve no máximo maxCandidates notas.
func (n *Notes) candidateNotes(ctx context.Context, userID int64, text string) []core.Note {
	if n.ai.Embedder == nil {
		return nil
	}
	qEmb, err := n.ai.Embedder.EmbedQuery(text)
	if err != nil {
		return nil
	}
	hits, err := n.store.SearchVector(ctx, userID, qEmb, maxCandidates)
	if err != nil {
		return nil
	}
	ids := make([]int64, 0, len(hits))
	for _, h := range hits {
		if h.Score >= attachMinSim {
			ids = append(ids, h.NoteID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	notes, err := n.store.NotesByIDs(ctx, userID, ids)
	if err != nil {
		return nil
	}
	byID := make(map[int64]core.Note, len(notes))
	for _, note := range notes {
		byID[note.ID] = note
	}
	out := make([]core.Note, 0, len(ids))
	for _, id := range ids {
		if note, ok := byID[id]; ok {
			out = append(out, note)
		}
	}
	return out
}

// validAttach resolve o attach_to do modelo contra os candidatos que ele
// de fato viu, impedindo IDs alucinados.
func validAttach(attachTo *int64, cands []core.Note) (core.Note, bool) {
	if attachTo == nil {
		return core.Note{}, false
	}
	for _, n := range cands {
		if n.ID == *attachTo {
			return n, true
		}
	}
	return core.Note{}, false
}

// buildAlertProposal converte o alert opcional do captureDecision em um
// core.AlertProposal, validando se o horário é futuro ou recorrente.
func (n *Notes) buildAlertProposal(raw json.RawMessage, fallbackTitle string) *core.AlertProposal {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var ad captureAlertDec
	if err := json.Unmarshal(raw, &ad); err != nil {
		return nil
	}
	msg := strings.TrimSpace(ad.Message)
	if msg == "" {
		msg = fallbackTitle
	}
	fireAt := strings.TrimSpace(ad.FireAt)
	rec := ""
	if len(ad.Recurrence) > 0 && string(ad.Recurrence) != "null" {
		rec = string(ad.Recurrence)
	}
	if !futureOrRecurring(fireAt, rec, time.Now()) {
		return nil
	}
	return &core.AlertProposal{Message: msg, FireAt: fireAt, Recurrence: rec}
}

// parseCaptureJSON faz o parsing do JSON devolvido pela IA, tratando
// possíveis cercas markdown ``` que o modelo pode incluir em qualquer posição.
func parseCaptureJSON(raw string) (captureDecision, error) {
	raw = extractJSON(raw)
	var dec captureDecision
	err := json.Unmarshal([]byte(raw), &dec)
	return dec, err
}

// extractJSON tenta extrair um bloco JSON de dentro de cercas markdown ```,
// ou retorna a string original se não encontrar. Lida com:
//
//	```json\n{"key":"value"}\n```
//	texto antes\n```\n{"key":"value"}\n```
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i != -1 {
			s = s[i+1:]
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	}
	if start := strings.Index(s, "\n```"); start != -1 {
		s = s[start+1:]
		if i := strings.IndexByte(s, '\n'); i != -1 {
			s = s[i+1:]
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	}
	return s
}

// normalizeTags normaliza as tags: minúsculas, sem espaços, sem duplicatas,
// máximo 5.
func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

// ResolveOverflow aplica a estratégia escolhida pelo usuário quando um append
// extrapolaria o limite de caracteres.
// mode: "part2" — cria uma nota irmã com sufixo "· parte N"
//       "summarize" — mescla e pede à IA para condensar
//       "split" — mescla e pede à IA para dividir em múltiplas notas temáticas
func (n *Notes) ResolveOverflow(ctx context.Context, userID, targetID int64, content string, tags []string, mode string) (core.CaptureResult, error) {
	content = strings.TrimSpace(content)
	note, err := n.store.GetNote(ctx, userID, targetID)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}

	switch mode {
	case "part2":
		return n.overflowPart2(ctx, userID, note.Title, content, unionTags(note.Tags, tags))
	case "summarize":
		return n.overflowSummarize(ctx, userID, note, content, tags)
	case "split":
		return n.overflowSplit(ctx, userID, note, content)
	default:
		return core.CaptureResult{Status: "error", Message: "unknown_mode"}, nil
	}
}

// overflowPart2 mantém a nota original intacta e cria uma nota irmã.
func (n *Notes) overflowPart2(ctx context.Context, userID int64, title, content string, tags []string) (core.CaptureResult, error) {
	return n.CreateFromProposal(ctx, userID, nextPartTitle(title), content, tags)
}

// overflowSummarize mescla o novo conteúdo na nota e substitui o corpo por um
// resumo de IA de tudo, cabendo no limite.
func (n *Notes) overflowSummarize(ctx context.Context, userID int64, note core.Note, content string, tags []string) (core.CaptureResult, error) {
	if !n.ai.hasKey() {
		return core.CaptureResult{Status: "no_api_key"}, nil
	}
	merged := strings.TrimSpace(note.Content) + "\n\n" + content
	language := n.loadLang(ctx, userID)
	summary, usage, err := n.ai.complete(ctx, buildNoteSummaryPrompt(core.Note{Title: note.Title, Content: merged}, lang(language)))
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, nil
	}
	mergedTags := unionTags(note.Tags, tags)
	updated, err := n.store.UpdateNote(ctx, userID, note.ID, note.Title, summary, mergedTags)
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, err
	}
	n.embedNote(ctx, userID, updated.ID, updated.Title, updated.Content)
	return core.CaptureResult{
		Status: "created", NoteID: updated.ID, NoteTitle: updated.Title,
		Content: updated.Content, Tags: updated.Tags, Usage: usage,
	}, nil
}

// overflowSplit mescla tudo e pede à IA para dividir em notas temáticas.
// Se a IA não devolver algo utilizável, cai para append simples.
func (n *Notes) overflowSplit(ctx context.Context, userID int64, note core.Note, content string) (core.CaptureResult, error) {
	if !n.ai.hasKey() {
		return core.CaptureResult{Status: "no_api_key"}, nil
	}
	merged := strings.TrimSpace(note.Content) + "\n\n" + content
	language := n.loadLang(ctx, userID)
	raw, usage, err := n.ai.complete(ctx, buildNoteSplitPrompt(note.Title, merged, lang(language)))
	if err != nil {
		return core.CaptureResult{Status: "error", Message: err.Error()}, nil
	}

	var dec struct {
		Notes []struct {
			Title   string   `json:"title"`
			Content string   `json:"content"`
			Tags    []string `json:"tags"`
		} `json:"notes"`
	}
	if jerr := json.Unmarshal([]byte(raw), &dec); jerr != nil || len(dec.Notes) < 2 {
		// fallback: manter tudo na nota original
		updated, uerr := n.store.UpdateNote(ctx, userID, note.ID, note.Title, merged, note.Tags)
		if uerr != nil {
			return core.CaptureResult{Status: "error", Message: uerr.Error()}, uerr
		}
		n.embedNote(ctx, userID, updated.ID, updated.Title, updated.Content)
		return core.CaptureResult{Status: "created", NoteID: updated.ID, NoteTitle: updated.Title, Content: updated.Content, Tags: updated.Tags, Usage: usage}, nil
	}

	var firstID int64
	created := 0
	for _, p := range dec.Notes {
		title := strings.TrimSpace(p.Title)
		body := strings.TrimSpace(p.Content)
		if body == "" {
			continue
		}
		if title == "" {
			title = extractTitle(body)
		}
		note, err := n.store.CreateNote(ctx, userID, title, body, normalizeTags(p.Tags), 0)
		if err != nil {
			return core.CaptureResult{Status: "error", Message: err.Error()}, err
		}
		n.embedNote(ctx, userID, note.ID, note.Title, note.Content)
		if firstID == 0 {
			firstID = note.ID
		}
		created++
	}
	// remove original
	_ = n.store.DeleteNote(ctx, userID, note.ID)
	return core.CaptureResult{Status: "created", NoteID: firstID, NoteTitle: note.Title, Usage: usage}, nil
}

// --- helpers de overflow ----------------------------------------------------

// overflowProposal retorna um OverflowProposal se o conteúdo mesclado exceder
// o limite informado, ou nil caso contrário.
func (n *Notes) overflowProposal(note core.Note, merged string, limit int) *core.OverflowProposal {
	if limit <= 0 {
		return nil
	}
	nChars := utf8.RuneCountInString(merged)
	if nChars <= limit {
		return nil
	}
	return &core.OverflowProposal{
		TargetID:    note.ID,
		TargetTitle: note.Title,
		Length:      nChars,
		Limit:       limit,
	}
}
// partTitleRe reconhece títulos que já terminam em "· parte N".
var partTitleRe = regexp.MustCompile(`^(.*?)\s*·\s*parte\s+(\d+)$`)

func nextPartTitle(title string) string {
	title = strings.TrimSpace(title)
	if m := partTitleRe.FindStringSubmatch(title); m != nil {
		n, _ := strconv.Atoi(m[2])
		return strings.TrimSpace(m[1]) + " · parte " + strconv.Itoa(n+1)
	}
	return title + " · parte 2"
}

// Summarize pede à IA um resumo curto da nota em Markdown. Não grava nada — só
// devolve o texto; quem aplica/desfaz é a camada acima (mantém o undo simétrico
// entre o comando e o botão da UI).
func (n *Notes) Summarize(ctx context.Context, userID, id int64) (core.SummarizeResult, error) {
	if !n.ai.hasKey() {
		return core.SummarizeResult{Status: "no_api_key"}, nil
	}
	note, err := n.store.GetNote(ctx, userID, id)
	if err != nil {
		return core.SummarizeResult{Status: "error", Message: err.Error()}, err
	}
	if strings.TrimSpace(note.Content) == "" {
		return core.SummarizeResult{Status: "empty"}, nil
	}
	language := n.loadLang(ctx, userID)
	summary, usage, err := n.ai.complete(ctx, buildNoteSummaryPrompt(note, lang(language)))
	if err != nil {
		return core.SummarizeResult{Status: "error", Message: err.Error()}, nil
	}
	return core.SummarizeResult{Status: "ok", Summary: summary, Usage: usage}, nil
}

// Tidy pede à IA para reorganizar a nota (estrutura/formatação) preservando TODA
// a informação — não resume. Como Summarize, só devolve o corpo novo; a aplicação
// (com undo) é da camada acima.
func (n *Notes) Tidy(ctx context.Context, userID, id int64) (core.TidyResult, error) {
	if !n.ai.hasKey() {
		return core.TidyResult{Status: "no_api_key"}, nil
	}
	note, err := n.store.GetNote(ctx, userID, id)
	if err != nil {
		return core.TidyResult{Status: "error", Message: err.Error()}, err
	}
	if strings.TrimSpace(note.Content) == "" {
		return core.TidyResult{Status: "empty"}, nil
	}
	language := n.loadLang(ctx, userID)
	content, usage, err := n.ai.complete(ctx, buildNoteTidyPrompt(note, lang(language)))
	if err != nil {
		return core.TidyResult{Status: "error", Message: err.Error()}, nil
	}
	return core.TidyResult{Status: "ok", Content: content, Usage: usage}, nil
}

// --- helpers ---------------------------------------------------------------

func unionTags(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range append(append([]string{}, a...), b...) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func shareTag(a, b []string) bool {
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	for _, t := range b {
		if set[t] {
			return true
		}
	}
	return false
}
