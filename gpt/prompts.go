package gpt

import (
	"github.com/alexanderi96/gptron/session"
	"github.com/sashabaranov/go-openai"
)

func MustBeApproved(engineID string) *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that their request to be whitelisted has been received and that they should await an administrator's review.",
			},
		},
	}
}

func CommandRestricted(engineID string) *openai.ChatCompletionRequest {

	return &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that the selected command is restricted to only admins.",
			},
		},
	}
}

func NewChat(engineID string) *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that he can now start a new conversation.",
			},
		},
	}
}

func GetTitleContext(engineID string, messages []*session.Message) *session.Conversation {
	ctx := &session.Conversation{
		Model: engineID,
		Content: []*session.Message{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: Personalities["TitleGenerator"],
			},
		},
	}
	for _, message := range messages {
		if message.Role != openai.ChatMessageRoleSystem {
			ctx.Content = append(ctx.Content, message)
		}
	}
	return ctx
}

func SummarizatorPrompt(engineID string, msg *[]openai.ChatCompletionMessage) *openai.ChatCompletionRequest {
	pers, _ := GetPersonalityWithCommonPrompts("ConversationalSynthesizer")

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: pers,
		},
	}

	messages = append(messages, *msg...)

	return &openai.ChatCompletionRequest{
		Model:    engineID,
		Messages: messages,
	}
}
