package service

import (
	"context"
	"testing"
	"time"

	"github.com/Pedro-0101/gix-server/internal/ai"
)

func TestCaptureNoAPIKey(t *testing.T) {
	aiDeps := AI{Client: ai.New("")}
	n := NewNotes(nil, aiDeps)
	res, err := n.Capture(context.Background(), 1, "testar captura")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if res.Status != "no_api_key" {
		t.Fatalf("status = %q, esperava no_api_key", res.Status)
	}
}

func TestCaptureEmptyText(t *testing.T) {
	aiDeps := AI{Client: ai.New("key")}
	n := NewNotes(nil, aiDeps)
	res, err := n.Capture(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("status = %q, esperava error", res.Status)
	}
}

func TestAskEmptyQuery(t *testing.T) {
	aiDeps := AI{Client: ai.New("key")}
	n := NewNotes(nil, aiDeps)
	res, err := n.Ask(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("status = %q, esperava error", res.Status)
	}
}

func TestLangDefault(t *testing.T) {
	if got := lang(""); got != "Português do Brasil" {
		t.Fatalf("lang('') = %q, esperava Português do Brasil", got)
	}
	if got := lang("English"); got != "English" {
		t.Fatalf("lang('English') = %q, esperava English", got)
	}
}

func TestParsePaginationDefaults(t *testing.T) {
	opts := ParseListOptions(0, -1)
	if opts.Limit != 50 {
		t.Fatalf("limit = %d, esperava 50", opts.Limit)
	}
	if opts.Offset != 0 {
		t.Fatalf("offset = %d, esperava 0", opts.Offset)
	}
}

func TestParsePaginationMaxLimit(t *testing.T) {
	opts := ParseListOptions(999, 10)
	if opts.Limit != 50 {
		t.Fatalf("limit = %d, esperava 50", opts.Limit)
	}
}

func TestExtractTitle(t *testing.T) {
	if got := extractTitle("Título principal\n\nconteúdo"); got != "Título principal" {
		t.Fatalf("extractTitle = %q", got)
	}
	if got := extractTitle("Curto"); got != "Curto" {
		t.Fatalf("extractTitle = %q", got)
	}
	if got := extractTitle(""); got != "Nota" {
		t.Fatalf("extractTitle vazio = %q", got)
	}
}

func TestFutureOrRecurring(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	if !futureOrRecurring("2026-06-30T10:00:00Z", "", now) {
		t.Fatal("futuro deveria ser válido")
	}
	if futureOrRecurring("2026-06-28T10:00:00Z", "", now) {
		t.Fatal("passado não-recorrente deveria ser inválido")
	}
	if !futureOrRecurring("2026-06-28T10:00:00Z", `{"freq":"daily"}`, now) {
		t.Fatal("passado recorrente deveria ser válido")
	}
}

func TestStripFences(t *testing.T) {
	if got := stripFences("```\nhello\n```"); got != "hello" {
		t.Fatalf("stripFences = %q", got)
	}
	if got := stripFences("no fences"); got != "no fences" {
		t.Fatalf("stripFences = %q", got)
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet("curto"); got != "curto" {
		t.Fatalf("snippet('curto') = %q", got)
	}
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	snip := snippet(long)
	if len([]rune(snip)) > snippetRunes+2 {
		t.Fatalf("snippet muito longo: %d runas", len([]rune(snip)))
	}
}

func TestNormalizeTags(t *testing.T) {
	tags := normalizeTags([]string{"  TAG1 ", "tag1", "TAG2", "", "tag3", "tag4", "tag5", "tag6"})
	if len(tags) > 5 {
		t.Fatalf("tags = %v, max 5", tags)
	}
	if tags[0] != "tag1" {
		t.Fatalf("primeira tag = %q, esperava tag1", tags[0])
	}
}

func TestUnionTags(t *testing.T) {
	u := unionTags([]string{"a", "b"}, []string{"b", "c"})
	if len(u) != 3 {
		t.Fatalf("unionTags = %v, esperava 3", u)
	}
}

func TestShareTag(t *testing.T) {
	if !shareTag([]string{"a", "b"}, []string{"b", "c"}) {
		t.Fatal("shareTag(['a','b'], ['b','c']) deveria ser true")
	}
	if shareTag([]string{"a"}, []string{"b"}) {
		t.Fatal("shareTag(['a'], ['b']) deveria ser false")
	}
}

func TestNextPartTitle(t *testing.T) {
	if got := nextPartTitle("Minha Nota"); got != "Minha Nota · parte 2" {
		t.Fatalf("nextPartTitle = %q", got)
	}
	if got := nextPartTitle("Minha Nota · parte 2"); got != "Minha Nota · parte 3" {
		t.Fatalf("nextPartTitle = %q", got)
	}
}




