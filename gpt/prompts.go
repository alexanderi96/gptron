package gpt

import (
	"github.com/sashabaranov/go-openai"
)

func MustBeApproved(model *Model) *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: model.Name,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that their request to be whitelisted has been received and that they should await an administrator's review.",
			},
		},
	}
}

func CommandRestricted(model *Model) *openai.ChatCompletionRequest {

	return &openai.ChatCompletionRequest{
		Model: model.Name,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that the selected command is restricted to only admins.",
			},
		},
	}
}

func NewChat(model *Model) *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: model.Name,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a polite sentence informing the user that he can now start a new conversation.",
			},
		},
	}
}

func GetTitleContext(model *Model, messages []openai.ChatCompletionMessage) *openai.ChatCompletionRequest {
	ctx := &openai.ChatCompletionRequest{
		Model: model.Name,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: HelperPersonalities["TitleGenerator"],
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

func SummarizatorPrompt(model *Model, messages []openai.ChatCompletionMessage) *openai.ChatCompletionRequest {
	ctx := &openai.ChatCompletionRequest{
		Model: model.Name,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: HelperPersonalities["ConversationalSynthesizer"],
			},
		},
	}

	content := ""

	for _, message := range messages {
		if message.Role != openai.ChatMessageRoleSystem {
			content += message.Role + ": " + message.Content + "\n\n"
		}
	}

	ctx.Messages = append(ctx.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: content,
	})

	return ctx
}
