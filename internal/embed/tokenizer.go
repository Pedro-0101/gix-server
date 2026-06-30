package embed

import (
	"encoding/json"
	"fmt"
	"os"
)

// --- HuggingFace tokenizer.json structures ----------------------------------

type hfConfig struct {
	Model hfModel `json:"model"`
}

type hfModel struct {
	Type     string          `json:"type"`
	Vocab    hfVocab         `json:"vocab"`
	UnkToken string          `json:"unk_token"`
}

type hfVocab map[string]int32

func (v *hfVocab) UnmarshalJSON(data []byte) error {
	var arr [][]interface{}
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*v = make(hfVocab, len(arr))
	for i, entry := range arr {
		if len(entry) < 1 {
			continue
		}
		tok, _ := entry[0].(string)
		(*v)[tok] = int32(i)
	}
	return nil
}

// Tokenizer implementa WordPiece tokenization lendo tokenizer.json da HF.
type Tokenizer struct {
	vocab map[string]int32
	ids   map[int32]string
	unkID int32
	clsID int32
	sepID int32
	padID int32
}

// NewTokenizer carrega o tokenizer.json e prepara o tokenizador.
func NewTokenizer(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg hfConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Model.Type != "WordPiece" {
		return nil, fmt.Errorf("tipo de tokenizer não suportado: %s", cfg.Model.Type)
	}
	t := &Tokenizer{
		vocab: cfg.Model.Vocab,
		ids:   reverseMap(cfg.Model.Vocab),
		unkID: lookup(cfg.Model.Vocab, cfg.Model.UnkToken, 100),
		clsID: lookup(cfg.Model.Vocab, "[CLS]", 101),
		sepID: lookup(cfg.Model.Vocab, "[SEP]", 102),
		padID: lookup(cfg.Model.Vocab, "[PAD]", 0),
	}
	return t, nil
}

// Encode tokeniza o texto com [CLS]..[SEP], truncado a maxLen.
func (t *Tokenizer) Encode(text string, maxLen int) []int32 {
	tokens := t.wordPieceTokenize(text)
	cap := maxLen - 2
	if cap <= 0 {
		return []int32{t.clsID, t.sepID}
	}
	if len(tokens) > cap {
		tokens = tokens[:cap]
	}
	out := make([]int32, 0, len(tokens)+2)
	out = append(out, t.clsID)
	out = append(out, tokens...)
	out = append(out, t.sepID)
	return out
}

// wordPieceTokenize aplica a tokenização completa (basic + wordpiece).
func (t *Tokenizer) wordPieceTokenize(text string) []int32 {
	words := basicTokenize(text)
	var ids []int32
	for _, w := range words {
		ids = append(ids, t.wordPiece(w)...)
	}
	return ids
}

// basicTokenize separa por whitespace + pontuação.
func basicTokenize(text string) []string {
	var words []string
	var cur []byte
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			flush()
		} else if isPunct(b) {
			flush()
			words = append(words, string(b))
		} else {
			cur = append(cur, b)
		}
	}
	flush()
	return words
}

func isPunct(b byte) bool {
	return (b >= 33 && b <= 47) || (b >= 58 && b <= 64) ||
		(b >= 91 && b <= 96) || (b >= 123 && b <= 126)
}

// wordPiece tenta a palavra inteira; falhando, tenta sub-tokens com ##.
func (t *Tokenizer) wordPiece(word string) []int32 {
	if id, ok := t.vocab[word]; ok {
		return []int32{id}
	}
	var pieces []int32
	runes := []rune(word)
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				pieces = append(pieces, id)
				found = true
				break
			}
			end--
		}
		if !found {
			pieces = append(pieces, t.unkID)
			start++
		} else {
			start = end
		}
	}
	return pieces
}

func reverseMap(m map[string]int32) map[int32]string {
	out := make(map[int32]string, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

func lookup(m map[string]int32, key string, fallback int32) int32 {
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}
