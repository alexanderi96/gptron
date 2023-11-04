package gpt

import (
	_ "embed"

	"context"
	"log"

	"github.com/alexanderi96/gptron/session"
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

func SendMessagesToChatGPT(ctx *session.Conversation) (*GptResponse, error) {
	resp, err := client.CreateChatCompletion(
		context.Background(),
		*ctx.GetChatCompletionRequest(),
	)

	if err != nil {
		log.Print("ChatCompletion error: ", err)
		return nil, err
	}

	return &GptResponse{resp.Choices[0].Message, resp.Usage}, nil
}

func SummarizeChat(conv *session.Conversation, n int) (string, error) {
	// Assicurati che ci siano abbastanza messaggi nella chat da riassumere
	if len(conv.Content) < n {
		log.Print("Not enough messages to summarize, using all messages")
		n = len(conv.Content)
	}

	convBk := conv
	convBk.Content = convBk.Content[len(conv.Content)-n:]

	ctx := convBk.GetChatCompletionRequest()

	// Invia la richiesta
	resp, err := client.CreateChatCompletion(
		context.Background(),
		*SummarizatorPrompt(DefaultGptEngine, &ctx.Messages),
	)

	if err != nil {
		log.Print("Completion error: ", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}
