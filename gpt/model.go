package gpt

import "github.com/sashabaranov/go-openai"

var (
	Models = map[string]Model{
		"gpt-3.5-turbo": {
			Name:    "gpt-3.5-turbo",
			Context: 4097,
			Pricing: Pricing{
				Prompt:     0.0015,
				Completion: 0.002,
			},
			Restricted: false,
		},
		"gpt-3.5-turbo-16k": {
			Name:    "gpt-3.5-turbo-16k",
			Context: 16385,
			Pricing: Pricing{
				Prompt:     0.003,
				Completion: 0.004,
			},
			Restricted: true,
		},
		"gpt-4": {
			Name:    "gpt-4",
			Context: 8192,
			Pricing: Pricing{
				Prompt:     0.03,
				Completion: 0.06,
			},
			Restricted: true,
		},
		"gpt-4-32k": {
			Name:    "gpt-4-32k",
			Context: 32768,
			Pricing: Pricing{
				Prompt:     0.06,
				Completion: 0.12,
			},
			Restricted: true,
		},
	}
)

// prices expressed in $$/1K tokens
type Pricing struct {
	Prompt     float32
	Completion float32
}

type Model struct {
	Name       string
	Context    int
	Pricing    Pricing
	Usage      openai.Usage
	Restricted bool
}

func (m *Model) GetCosts() *Pricing {
	return &Pricing{
		Prompt:     float32(m.Usage.PromptTokens) * m.Pricing.Prompt / 1000,
		Completion: float32(m.Usage.CompletionTokens) * m.Pricing.Completion / 1000,
	}

}
