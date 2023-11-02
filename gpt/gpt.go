package gpt

import (
	_ "embed"

	"context"
	"log"

	"github.com/sashabaranov/go-openai"
)

type Model struct {
	EngineID string
	// parameters in order to keep track of the costs
}

var (
	//go:embed openai_api_key
	openaiApiKey string

	client *openai.Client
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
		log.Print("ListEngines error: ", err)
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
		log.Print("ChatCompletion error: ", err)
		return "", err
	}

	return resp.Text, nil
}

func SendMessagesToChatGPT(ctx openai.ChatCompletionRequest) (string, error) {

	resp, err := client.CreateChatCompletion(
		context.Background(),
		ctx,
	)

	if err != nil {
		log.Print("ChatCompletion error: ", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func SendTextToChatGPT(message *openai.CompletionRequest) (string, error) {
	resp, err := client.CreateCompletion(
		context.Background(),
		*message,
	)

	if err != nil {
		log.Print("Completion error: ", err)
		return "", err
	}
	return resp.Choices[0].Text, nil
}
