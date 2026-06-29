package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const endpoint = "https://openrouter.ai/api/v1/chat/completions"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

func New(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{},
		apiKey:     apiKey,
		baseURL:    endpoint,
	}
}

// HasKey informa se há uma chave configurada. As intents usam isto para devolver
// status "no_api_key" sem disparar uma chamada que falharia com 401.
func (c *Client) HasKey() bool { return c.apiKey != "" }

// Tool descreve uma função que o modelo pode chamar (tool calling).
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall é uma chamada de função completa, remontada a partir do stream.
type ToolCall struct {
	Name      string
	Arguments string
}

type toolCallAcc struct {
	name string
	args strings.Builder
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// Usage contém a contagem de tokens retornada pela API.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type completion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

// Complete faz uma chamada não-streaming (stream:false) e retorna o conteúdo
// inteiro da primeira choice mais o Usage. Usado para respostas estruturadas
// (JSON) como o roteamento de notas. Status != 2xx vira erro com o corpo.
func (c *Client) Complete(ctx context.Context, model string, messages []Message) (string, *Usage, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "gix")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var parsed completion
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", nil, err
	}
	var content string
	if len(parsed.Choices) > 0 {
		content = parsed.Choices[0].Message.Content
	}
	return content, parsed.Usage, nil
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int `json:"index"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

// Stream mantém a interface antiga: stream de texto, sem ferramentas.
func (c *Client) Stream(ctx context.Context, model string, messages []Message, onDelta func(string)) (*Usage, error) {
	u, _, err := c.StreamTools(ctx, model, messages, nil, onDelta)
	return u, err
}

// StreamTools faz a chamada com stream:true, repassa texto via onDelta e remonta
// quaisquer tool_calls (argumentos chegam em pedaços por índice). Retorna o Usage
// e as tool calls completas. ctx cancelado aborta; status != 2xx vira erro.
func (c *Client) StreamTools(ctx context.Context, model string, messages []Message, tools []Tool, onDelta func(string)) (*Usage, []ToolCall, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: messages, Stream: true, Tools: tools})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "gix")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var usage *Usage
	calls := map[int]*toolCallAcc{}
	var order []int

	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "[DONE]" {
				return usage, collectToolCalls(calls, order), nil
			}
			var chunk streamChunk
			if json.Unmarshal([]byte(data), &chunk) == nil {
				if chunk.Usage != nil {
					usage = chunk.Usage
				}
				for _, ch := range chunk.Choices {
					if ch.Delta.Content != "" {
						onDelta(ch.Delta.Content)
					}
					for _, tc := range ch.Delta.ToolCalls {
						a, ok := calls[tc.Index]
						if !ok {
							a = &toolCallAcc{}
							calls[tc.Index] = a
							order = append(order, tc.Index)
						}
						if tc.Function.Name != "" {
							a.name = tc.Function.Name
						}
						a.args.WriteString(tc.Function.Arguments)
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return usage, collectToolCalls(calls, order), nil
			}
			return usage, collectToolCalls(calls, order), readErr
		}
	}
}

func collectToolCalls(calls map[int]*toolCallAcc, order []int) []ToolCall {
	if len(order) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(order))
	for _, idx := range order {
		a := calls[idx]
		out = append(out, ToolCall{Name: a.name, Arguments: a.args.String()})
	}
	return out
}
