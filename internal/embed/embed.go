// Package embed gera embeddings de texto usando ONNX (e5-small, 384 dims).
// Modelo: Xenova/multilingual-e5-small int8.
// Pipeline: tokenizer -> ONNX -> mean pool -> L2 normalize.
//
// CGO_ENABLED=1 + libonnxruntime.so/dll são necessários em tempo de execução.
package embed

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	Dim    = 384 // dimensionalidade do e5-small
	MaxLen = 128 // comprimento máximo de tokens
)

// Embedding é um vetor normalizado de 384 floats.
type Embedding []float32

// Embedder gera embeddings a partir de texto usando ONNX.
type Embedder struct {
	session *ort.DynamicAdvancedSession
	tok     *Tokenizer
}

// NewEmbedder carrega o modelo ONNX e o tokenizer do diretório modelDir.
// modelDir deve conter model_quantized.onnx e tokenizer.json.
func NewEmbedder(modelDir string) (*Embedder, error) {
	tok, err := NewTokenizer(filepath.Join(modelDir, "tokenizer.json"))
	if err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}
	if _, err := os.Stat(filepath.Join(modelDir, "model_quantized.onnx")); err != nil {
		return nil, fmt.Errorf("modelo não encontrado: %w", err)
	}

	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := ort.NewDynamicAdvancedSession(
		filepath.Join(modelDir, "model_quantized.onnx"),
		inputNames, outputNames, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("onnx session: %w", err)
	}

	return &Embedder{session: session, tok: tok}, nil
}

// Close libera a sessão ONNX.
func (e *Embedder) Close() error {
	return e.session.Destroy()
}

// EmbedQuery gera embedding para consulta (prefixo "query: ").
func (e *Embedder) EmbedQuery(text string) (Embedding, error) {
	return e.run("query: " + text)
}

// EmbedPassage gera embedding para armazenamento (prefixo "passage: ").
func (e *Embedder) EmbedPassage(text string) (Embedding, error) {
	return e.run("passage: " + text)
}

func (e *Embedder) run(text string) (Embedding, error) {
	tokens := e.tok.Encode(text, MaxLen)
	n := int64(len(tokens))

	inputIDs := make([]int64, n)
	mask := make([]int64, n)
	ttype := make([]int64, n)
	for i := range tokens {
		inputIDs[i] = int64(tokens[i])
		mask[i] = 1
	}

	shape := ort.NewShape(1, n)
	inT, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("input tensor: %w", err)
	}
	defer inT.Destroy()

	maT, err := ort.NewTensor(shape, mask)
	if err != nil {
		return nil, fmt.Errorf("mask tensor: %w", err)
	}
	defer maT.Destroy()

	tyT, err := ort.NewTensor(shape, ttype)
	if err != nil {
		return nil, fmt.Errorf("type tensor: %w", err)
	}
	defer tyT.Destroy()

	outShape := ort.NewShape(1, n, Dim)
	outData := make([]float32, n*Dim)
	outT, err := ort.NewTensor(outShape, outData)
	if err != nil {
		return nil, fmt.Errorf("output tensor: %w", err)
	}
	defer outT.Destroy()

	if err := e.session.Run(
		[]ort.Value{inT, maT, tyT},
		[]ort.Value{outT},
	); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	hidden := make([][]float32, n)
	for i := range hidden {
		hidden[i] = outData[i*Dim : (i+1)*Dim]
	}
	return meanPoolNormalize(hidden), nil
}

// InitEnvironment inicializa o runtime ONNX. Chamar uma vez no boot.
func InitEnvironment() error { return ort.InitializeEnvironment() }

// --- vector ops ------------------------------------------------------------

// Cosine retorna a similaridade de cosseno entre dois embeddings normalizados
// (dot product, já que estão L2-normalizados).
func Cosine(a, b Embedding) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

// meanPoolNormalize faz mean pool + L2 normalize.
func meanPoolNormalize(hidden [][]float32) Embedding {
	out := make([]float32, Dim)
	if len(hidden) == 0 {
		return out
	}
	for _, h := range hidden {
		for i, v := range h {
			out[i] += v
		}
	}
	n := float32(len(hidden))
	for i := range out {
		out[i] /= n
	}
	var sum float64
	for _, v := range out {
		sum += float64(v) * float64(v)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range out {
			out[i] /= norm
		}
	}
	return out
}
