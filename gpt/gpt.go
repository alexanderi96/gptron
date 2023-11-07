package gpt

import (
	_ "embed"

	"context"
	"log"

	"github.com/sashabaranov/go-openai"
)

type GptResponse struct {
	Message openai.ChatCompletionMessage
	Usage   openai.Usage
}

var (
	//go:embed openai_api_key
	openaiApiKey string

	client *openai.Client

	DefaultGptEngine = openai.GPT4
)

func init() {

	if len(openaiApiKey) == 0 {
		log.Fatal("openai_api_key not set")
	}

	client = openai.NewClient(openaiApiKey)
}

func ListAvailableModels() (*openai.EnginesList, error) {
	engines, err := client.ListEngines(context.Background())
	if err != nil {
		return &openai.EnginesList{}, err
	}
	return &engines, nil
}

func SendVoiceToWhisper(voicePath string) (string, error) {
	resp, err := client.CreateTranscription(context.Background(), openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: voicePath,
	})

	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

func SendMessagesToChatGPT(ctx *openai.ChatCompletionRequest) (*GptResponse, error) {

	resp, err := client.CreateChatCompletion(
		context.Background(),
		*ctx,
	)

	if err != nil {
		log.Print("ChatCompletion error: ", err)
		return nil, err
	}

	return &GptResponse{resp.Choices[0].Message, resp.Usage}, nil
}
