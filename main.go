package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NicoNex/echotron/v3"

	openai "github.com/sashabaranov/go-openai"
)

type UserStatus int

const (
	Unreviewed UserStatus = iota
	Whitelisted
	Blacklisted
)

type WhisperResponse struct {
	Transcript string `json:"transcript"`
}

type User struct {
	Status UserStatus
}

type Conversation struct {
	Messages []string
}

var userConversations map[int64]*Conversation

type bot struct {
	chatID int64
	echotron.API
}

var (
	//go:embed telegram_token
	telegramToken string

	//go:embed openai_api_key
	openaiApiKey string

	//go:embed elevenlabs_api_key
	elevenlabsApiKey string

	//go:embed admin
	admin string

	//dsp *echotron.Dispatcher

	client *openai.Client

	commands = []echotron.BotCommand{
		{Command: "/start", Description: "start bot"},
		{Command: "/ping", Description: "check bot status"},
		{Command: "/help", Description: "help"},
		{Command: "/whitelist", Description: "whitelist user"},
		{Command: "/blacklist", Description: "blacklist user"},
	}

	users map[int64]*User

	parseMarkdown = &echotron.MessageOptions{ParseMode: echotron.Markdown}

	isNumber = regexp.MustCompile(`^\d+$`).MatchString
)

func init() {
	if len(telegramToken) == 0 {
		log.Fatal("Empty telegram_token file")
	}

	if len(admin) == 0 {
		log.Fatal("Empty admin file")
	}

	if len(openaiApiKey) == 0 {
		log.Fatal("Empty openai_api_key file")
	}

	if len(elevenlabsApiKey) == 0 {
		log.Fatal("Empty elevenlabs_api_key file")
	}

	client = openai.NewClient(openaiApiKey)

	go setCommands()

	err := loadUsers("users.json")
	if err != nil {
		log.Fatalf("failed to load users: %v", err)
	}

	userConversations = make(map[int64]*Conversation)
}

func saveUsers(filePath string) error {

	jsonData, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal users: %w", err)
	}

	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func loadUsers(filePath string) error {

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		users = make(map[int64]*User)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	err = json.Unmarshal(jsonData, &users)
	if err != nil {
		return fmt.Errorf("failed to unmarshal users: %w", err)
	}

	return nil
}

func updateConversation(chatID int64, message string) {
	conversation, exists := userConversations[chatID]
	if !exists {
		conversation = &Conversation{}
		userConversations[chatID] = conversation
	}
	conversation.Messages = append(conversation.Messages, message)
}

func setCommands() {
	api := echotron.NewAPI(telegramToken)
	api.SetMyCommands(nil, commands...)
}

func newBot(chatID int64) echotron.Bot {
	bot := &bot{
		chatID,
		echotron.NewAPI(telegramToken),
	}
	//go bot.selfDestruct(time.After(time.Hour))
	return bot
}

func message(update *echotron.Update) string {
	if update == nil {
		return ""
	} else if update.Message != nil {
		return update.Message.Text
	} else if update.EditedMessage != nil {
		return update.EditedMessage.Text
	} else if update.CallbackQuery != nil {
		return update.CallbackQuery.Data
	}
	return ""
}

func (b *bot) Update(update *echotron.Update) {
	msg := message(update)
	replyWithVoice := false
	isAdmin := strconv.Itoa(int(b.chatID)) == admin
	log.Printf("Message recieved from: " + strconv.FormatInt(b.chatID, 10))

	user, exists := users[b.chatID]
	if !exists && !isAdmin {

		user = &User{Status: Unreviewed}
		users[b.chatID] = user
		b.notifyAdmin(b.chatID)
		b.SendMessage("Your request to be whitelisted has been received, please wait for an admin to review it", b.chatID, parseMarkdown)

		return

	} else if !isAdmin {
		switch user.Status {
		case Unreviewed:

			b.SendMessage("👀", b.chatID, parseMarkdown)
			return
		case Blacklisted:

			b.SendMessage("💀", b.chatID, parseMarkdown)

			return
		case Whitelisted:

		default:
		}
	}

	if update.Message != nil && update.Message.Voice != nil {
		var err error

		replyWithVoice = true

		if msg, err = b.transcript(update.Message.Voice.FileID); err != nil {
			log.Printf("Error transcribing message: %x", err)
			b.SendMessage("Error transcribing message", b.chatID, parseMarkdown)
		}

	}

	switch {
	case strings.HasPrefix(msg, "/ping"):
		b.SendMessage("pong", b.chatID, parseMarkdown)

	case strings.HasPrefix(msg, "/whitelist"):
		if !isAdmin {
			b.SendMessage("Only admins can use this command", b.chatID, parseMarkdown)
			return
		}
		slice := strings.Split(msg, " ")

		if len(slice) != 2 && isNumber(slice[1]) {
			b.SendMessage("Invalid chat ID: "+slice[1], b.chatID, parseMarkdown)
			return
		}
		userChatID, _ := strconv.Atoi(slice[1])

		if users[int64(userChatID)].Status == Whitelisted {
			b.SendMessage("User "+slice[1]+" already whitelisted", b.chatID, parseMarkdown)
			return
		}
		users[int64(userChatID)].Status = Whitelisted
		b.SendMessage("You have been whitelisted", int64(userChatID), parseMarkdown)

		saveUsers("users.json")
		b.SendMessage("User "+slice[1]+" has been whitelisted", b.chatID, parseMarkdown)

	case strings.HasPrefix(msg, "/blacklist"):
		if !isAdmin {
			b.SendMessage("Only admins can use this command", b.chatID, parseMarkdown)
			return
		}
		slice := strings.Split(msg, " ")

		if len(slice) != 2 && isNumber(slice[1]) {
			b.SendMessage("Invalid chat ID: "+slice[1], b.chatID, parseMarkdown)
			return
		}
		userChatID, _ := strconv.Atoi(slice[1])

		if users[int64(userChatID)].Status == Blacklisted {
			b.SendMessage("User "+slice[1]+" already blacklisted", b.chatID, parseMarkdown)
			return
		}
		users[int64(userChatID)].Status = Blacklisted
		b.SendMessage("You have been blacklisted", int64(userChatID), parseMarkdown)

		saveUsers("users.json")
		b.SendMessage("User "+slice[1]+" has been blacklisted", b.chatID, parseMarkdown)

	default:

		initialMessage, err := b.SendMessage("Analizing message...", b.chatID, parseMarkdown)
		if err != nil {
			log.Printf("Error sending initial message to %d: %x", b.chatID, err)
			return
		}

		initialMessageID := echotron.NewMessageID(b.chatID, initialMessage.Result.ID)

		log.Printf("Sending %d's message to ChatGPT", b.chatID)
		response, err := SendMessageToChatGPT(b.chatID, msg, "gpt-4")
		if err != nil {
			errorMessage := fmt.Errorf("error contacting ChatGPT: %x", err)
			log.Println(errorMessage.Error())
			b.EditMessageText(errorMessage.Error(), initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})

		} else {
			log.Printf("Sending response to %d", b.chatID)

			if replyWithVoice {
				audioLocation, err := textToSpeech(response)
				if err != nil {
					errorMessage := fmt.Errorf("error generating speech from text: %x", err)
					log.Println(errorMessage.Error())
					b.EditMessageText(errorMessage.Error(), initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
				}
				b.SendVoice(echotron.NewInputFilePath(audioLocation), b.chatID, nil)
			} else {

				b.EditMessageText(response, initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
			}
		}
	}
}

func (b *bot) notifyAdmin(chatID int64) {

	message := fmt.Sprintf(
		"Nuovo utente non revisionato: %d", chatID,
	)

	adminID, err := strconv.ParseInt(admin, 10, 64)
	if err != nil {
		log.Printf("Failed to convert admin ID: %v", err)
		return
	}

	_, err = b.SendMessage(message, adminID, &echotron.MessageOptions{ReplyMarkup: echotron.InlineKeyboardMarkup{
		InlineKeyboard: [][]echotron.InlineKeyboardButton{
			{
				echotron.InlineKeyboardButton{Text: "Whitelist", CallbackData: fmt.Sprintf("/whitelist %d", chatID)},
				echotron.InlineKeyboardButton{Text: "Blacklist", CallbackData: fmt.Sprintf("/blacklist %d", chatID)},
			},
		},
	}})
	if err != nil {
		log.Printf("Failed to send message to admin: %v", err)
	}
}

func (b *bot) transcript(fileID string) (string, error) {
	log.Printf("Transcribing message received from: " + strconv.FormatInt(b.chatID, 10))

	fileMetadata, err := b.GetFile(fileID)

	if err != nil {
		return "", fmt.Errorf("error getting file Metadata: %x", err)
	}

	file, err := b.DownloadFile(fileMetadata.Result.FilePath)
	if err != nil {
		return "", fmt.Errorf("error downloading file to be transcripted: %x", err)
	}

	transcript, err := sendVoiceToWhisper(file)
	if err != nil {
		return "", fmt.Errorf("error sending voice to Whisper: %x", err)
	}
	return transcript, nil
}
func sendVoiceToWhisper(voiceData []byte) (string, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fw, err := w.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(fw, bytes.NewReader(voiceData))
	if err != nil {
		return "", err
	}

	err = w.WriteField("model", "whisper-1")
	if err != nil {
		return "", err
	}

	err = w.Close()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &b)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+openaiApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&response)
	if err != nil {
		return "", err
	}

	transcription, ok := response["text"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format")
	}

	return transcription, nil
}

type ElevenLabsResponse struct {
	AudioURL string `json:"audio_url"`
}

func textToSpeech(text string) (string, error) {
	log.Printf("Converting text to speech")
	url := "https://api.elevenlabs.io/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM?optimize_streaming_latency=0&output_format=mp3_44100_128"

	requestBody := map[string]interface{}{
		"text":     text,
		"model_id": "eleven_multilingual_v2",
		"voice_settings": map[string]interface{}{
			"stability":         0.5,
			"similarity_boost":  0,
			"style":             0,
			"use_speaker_boost": true,
		},
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", elevenlabsApiKey)
	req.Header.Set("accept", "audio/mpeg")

	client := &http.Client{}
	resp, err := client.Do(req)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code: %s", resp.Status)

	} else if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp("", "tts_audio_*.mp3")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	_, err = tempFile.Write(audioData)
	if err != nil {
		return "", err
	}
	log.Printf("Audio saved")
	return tempFile.Name(), nil
}

func SendMessageToChatGPT(chatID int64, message string, engineID string) (string, error) {
	updateConversation(chatID, message)
	conversation := userConversations[chatID]

	var messages []openai.ChatCompletionMessage
	for _, msg := range conversation.Messages {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: msg,
		})
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    engineID,
			Messages: messages,
		},
	)

	if err != nil {
		log.Print("ChatCompletion error: ", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func main() {
	dsp := echotron.NewDispatcher(telegramToken, newBot)
	log.Printf("Running GPTronBot...")

	for {
		log.Println(dsp.Poll())
		log.Printf("Lost connection, waiting one minute...")

		time.Sleep(1 * time.Minute)
	}
}
