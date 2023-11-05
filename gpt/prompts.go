package gpt

import (
	"log"

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

func GetTitleContext(engineID string, messages []openai.ChatCompletionMessage) *openai.ChatCompletionRequest {
	log.Println("Generating title for context...")
	ctx := &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: Personalities["TitleGenerator"],
			},
		},
	}
	for _, message := range messages {
		if message.Role != openai.ChatMessageRoleSystem {
			ctx.Messages = append(ctx.Messages, message)
		}
	}
	log.Println("%v", ctx)
	return ctx
}

func SummarizatorPrompt(engineID string, messages []openai.ChatCompletionMessage) *openai.ChatCompletionRequest {
	ctx := &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: Personalities["ConversationalSynthesizer"],
			},
		},
	}

	for _, message := range messages {
		if message.Role != openai.ChatMessageRoleSystem {
			ctx.Messages = append(ctx.Messages, message)
		}
	}

	return ctx
}
