package gpt

import "github.com/sashabaranov/go-openai"

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

func GetTitle(engineID, msg string) *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: engineID,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "Craft a max 10 word title for the following message:\n\n" + msg,
			},
		},
	}
}
