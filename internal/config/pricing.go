package config

// ModelPricing representa o custo por 1M de tokens (USD) no OpenRouter.
type ModelPricing struct {
	InputPrice  float64
	OutputPrice float64
}

// CalculateCost calcula o custo em dólares a partir do uso de tokens.
func (p ModelPricing) CalculateCost(promptTokens, completionTokens int) float64 {
	inputCost := p.InputPrice * float64(promptTokens) / 1_000_000
	outputCost := p.OutputPrice * float64(completionTokens) / 1_000_000
	return inputCost + outputCost
}

// ModelPrices mapeia cada modelo ao seu custo por 1M tokens (USD). Modelo
// ausente => custo 0 (só a contagem de tokens é reportada).
var ModelPrices = map[string]ModelPricing{
	"google/gemini-2.5-flash-lite":       {0.075, 0.30},
	"google/gemini-2.5-flash":            {0.10, 0.40},
	"google/gemini-2.5-pro":              {1.25, 5.00},
	"openai/gpt-4o":                      {2.50, 10.00},
	"openai/gpt-4o-mini":                 {0.15, 0.60},
	"openai/o3-mini":                     {1.10, 4.40},
	"anthropic/claude-sonnet-4-20250514": {3.00, 15.00},
	"anthropic/claude-3.5-haiku":         {0.80, 4.00},
	"deepseek/deepseek-chat":             {0.27, 1.10},
	"deepseek/deepseek-r1":               {0.55, 2.19},
	"meta-llama/llama-3.3-70b-instruct":  {0.25, 0.25},
	"mistral/mistral-large":              {2.00, 6.00},
}
