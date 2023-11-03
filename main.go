package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NicoNex/echotron/v3"
	"github.com/alexanderi96/gptron/elevenlabs"
	"github.com/alexanderi96/gptron/gpt"
	"github.com/alexanderi96/gptron/session"
	"github.com/alexanderi96/gptron/utils"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

type bot struct {
	chatID int64
	echotron.API
	Users map[int64]*session.User
}

var (
	//go:embed telegram_token
	telegramToken string

	//go:embed admin
	admin string

	dsp *echotron.Dispatcher

	commands = []echotron.BotCommand{
		{Command: "/start", Description: "start bot"},
		{Command: "/list", Description: "show all conversations"},
		{Command: "/select", Description: "select conversation"},
		{Command: "/ping", Description: "check bot status"},
		{Command: "/help", Description: "help"},
		{Command: "/whitelist", Description: "whitelist user"},
		{Command: "/blacklist", Description: "blacklist user"},
	}

	replyWithVoice = false
)

func init() {
	if len(telegramToken) == 0 {
		log.Fatal("telegram_token not set")
	}

	if len(admin) == 0 {
		log.Fatal("admin not set")
	}

	dsp = echotron.NewDispatcher(telegramToken, newBot)
	//go setCommands()

}

func newBot(chatID int64) echotron.Bot {
	bot := &bot{
		chatID,
		echotron.NewAPI(telegramToken),
		make(map[int64]*session.User),
	}

	err := bot.loadUsers("users.json")
	if err != nil {
		log.Fatalf("Failed to load user list: %v", err)
	}

	//go bot.selfDestruct(time.After(time.Hour))
	return bot
}

func setCommands() {
	api := echotron.NewAPI(telegramToken)
	api.SetMyCommands(nil, commands...)
}

func main() {
	log.Printf("Running GPTronBot...")

	for {
		log.Println(dsp.Poll())
		log.Printf("Lost connection, waiting one minute...")

		time.Sleep(1 * time.Minute)
	}
}

func (b *bot) handleNewUser() {
	user := session.NewUser(strconv.Itoa(int(b.chatID)) == admin, b.chatID)

	b.Users[b.chatID] = user
	b.notifyAdmin(b.chatID)

	b.SendMessage("Your request to be whitelisted has been received, please wait for an admin to review it", b.chatID, nil)
}

func (b *bot) handleUserApproval(msg string, user *session.User) {
	if !user.IsAdmin {
		b.SendMessage("Only admins can use this command", b.chatID, nil)
		return
	}
	slice := strings.Split(msg, " ")

	if len(slice) != 2 && utils.IsNumber(slice[1]) {
		b.SendMessage("Invalid chat ID: "+slice[1], b.chatID, nil)
		return
	}
	userChatID, _ := strconv.Atoi(slice[1])
	if slice[0] == "/whitelist" {
		if b.Users[int64(userChatID)].Status == session.Whitelisted {
			b.SendMessage("User "+slice[1]+" already whitelisted", b.chatID, nil)
			return
		}
		b.Users[int64(userChatID)].Status = session.Whitelisted
		b.SendMessage("You have been whitelisted", int64(userChatID), nil)
	} else if slice[0] == "/blacklist" {
		if b.Users[int64(userChatID)].Status == session.Blacklisted {
			b.SendMessage("User "+slice[1]+" already blacklisted", b.chatID, nil)
			return
		}
		b.Users[int64(userChatID)].Status = session.Blacklisted
		b.SendMessage("You have been blacklisted", int64(userChatID), nil)
	}
}

func (b *bot) handleSelect(msg string) {
	slice := strings.Split(msg, " ")

	if len(slice) < 2 && utils.IsUUID(slice[len(slice)-1]) {
		b.SendMessage("Invalid chat ID", b.chatID, nil)
		return
	}

	argID, _ := uuid.Parse(slice[len(slice)-1])

	if b.Users[b.chatID].Conversations[argID] == nil {
		b.SendMessage("Conversation "+argID.String()+" not found", b.chatID, nil)
		return
	}
	b.Users[b.chatID].MenuState = session.MenuStateSelected

	log.Printf("User %d selected conversation %s", b.chatID, argID.String())
	b.SendMessage("Switched to conversation "+argID.String(), b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getConversationUI(), ParseMode: echotron.Markdown})

	b.Users[b.chatID].SelectedConversation = argID
}
func (b *bot) Update(update *echotron.Update) {
	log.Printf("New command from " + strconv.FormatInt(b.chatID, 10))

	msg := message(update)

	user, exists := b.Users[b.chatID]
	if !exists {
		b.handleNewUser()
	} else if !user.IsAdmin {
		switch user.Status {
		case session.Unreviewed:
			b.SendMessage("👀", b.chatID, nil)
			return

		case session.Blacklisted:
			b.SendMessage("💀", b.chatID, nil)
			return

		case session.Whitelisted:
		default:
		}
	}

	user.LastUpdate = time.Now()
	if user.MenuState == "" {
		user.MenuState = session.MenuStateMain
	}

	switch {
	case strings.HasPrefix(msg, "/ping"):
		b.SendMessage("pong", b.chatID, nil)

	case strings.HasPrefix(msg, "/whitelist"), strings.HasPrefix(msg, "/blacklist"):
		b.handleUserApproval(msg, user)

	case strings.HasPrefix(msg, "/list"):
		user.MenuState = session.MenuStateList
		b.SendMessage(
			"Select a conversation from the list or start a new one",
			b.chatID,
			&echotron.MessageOptions{
				ReplyMarkup: b.getListOfChats(),
				ParseMode:   echotron.Markdown,
			},
		)

	case strings.HasPrefix(msg, "/select"):
		b.handleSelect(msg)

	case strings.HasPrefix(msg, "/back"):
		if user.MenuState == session.MenuStateSelected {
			user.MenuState = session.MenuStateList
			b.SendMessage("Select a conversation from the list", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getListOfChats(), ParseMode: echotron.Markdown})
		} else {
			user.MenuState = session.MenuStateMain
			b.SendMessage("Main menu", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMainMenu(), ParseMode: echotron.Markdown})
		}

	case strings.HasPrefix(msg, "/home"):
		user.MenuState = session.MenuStateMain
		b.SendMessage("Main menu", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMainMenu(), ParseMode: echotron.Markdown})

	case strings.HasPrefix(msg, "/new"):

		user.CreateNewConversation(gpt.DefaultGptEngine, strconv.Itoa(int(b.chatID)))
		user.MenuState = session.MenuStateSelected

		if user.Conversations[user.SelectedConversation].GptPersonality == "" {
			user.MenuState = session.MenuStateSelectPersonality
			b.SendMessage("Select a personality from the list", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getPersonalityList(), ParseMode: echotron.Markdown})
		}

	case strings.HasPrefix(msg, "/stats"):
		if user.MenuState == session.MenuStateMain {
			b.SendMessage(user.GetGlobalStats(), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		} else if user.MenuState == session.MenuStateSelected {
			b.SendMessage(user.GetConversationStats(user.SelectedConversation), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}

	case strings.HasPrefix(msg, "/summarize"):
		if user.MenuState == session.MenuStateSelected {
			rsp, err := gpt.SummarizeChat(user.Conversations[user.SelectedConversation], 10)
			if err != nil {
				b.SendMessage(err.Error(), b.chatID, nil)
			} else {
				b.SendMessage(rsp, b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
			}
		}

	case strings.HasPrefix(msg, "/ask"):
		if user.MenuState == session.MenuStateSelectPersonality {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				log.Printf("User %d sent an invalid input: %s", b.chatID, msg)
				b.SendMessage("Invalid input", b.chatID, nil)
				return
			}

			if gpt.Personalities[slice[1]] == "" {
				log.Printf("User %d asked for personality %s but it does not exist: %s", b.chatID, slice[1], msg)
				b.SendMessage("Personality "+slice[1]+" not found", b.chatID, nil)
				return
			} else {
				user.Conversations[user.SelectedConversation].GptPersonality = slice[1]
				pers, _ := gpt.GetPersonalityWithCommonPrompts(slice[1])
				user.Conversations[user.SelectedConversation].AppendMessage(pers, openai.ChatMessageRoleSystem)
				log.Printf("User %d selected personality %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].GptPersonality, user.SelectedConversation)
				b.SendMessage("Selected personality "+slice[1]+"\nYou may now start talking with ChatGPT.", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getConversationUI(), ParseMode: echotron.Markdown})
				user.MenuState = session.MenuStateSelected
			}

		}

	case strings.HasPrefix(msg, "/delete"):
		if user.MenuState == session.MenuStateSelected && user.SelectedConversation != uuid.Nil {
			convID := user.SelectedConversation

			if user.Conversations[convID] == nil {
				b.SendMessage("Conversation "+convID.String()+" not found", b.chatID, nil)
				return
			}
			user.MenuState = session.MenuStateMain

			delete(user.Conversations, convID)
			user.SelectedConversation = uuid.Nil

			log.Printf("User %d deleted conversation: %s", b.chatID, convID)
			b.SendMessage("Conversation "+convID.String()+" has been deleted", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMainMenu(), ParseMode: echotron.Markdown})

		}

	default:
		if user.MenuState != session.MenuStateSelected {
			b.SendMessage("Select a conversation from the list or start a new one", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMainMenu()})
			return
		}
		if user.Conversations[user.SelectedConversation].GptPersonality == "" {
			user.MenuState = session.MenuStateSelectPersonality
			b.SendMessage("Select a personality from the list", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getPersonalityList(), ParseMode: echotron.Markdown})
			return
		}

		b.handleCommunication(user, msg, update)
		b.Users[b.chatID] = user
	}
	b.saveUsers("users.json")

}

func (b *bot) handleCommunication(user *session.User, msg string, update *echotron.Update) {
	selectedConversation := user.Conversations[user.SelectedConversation]

	initialMessage, err := b.SendMessage("Analizing message...", b.chatID, nil)
	if err != nil {
		log.Printf("Error sending initial message to %d: %s", b.chatID, err)
		return
	}

	initialMessageID := echotron.NewMessageID(b.chatID, initialMessage.Result.ID)

	if update.Message != nil && update.Message.Voice != nil {
		var err error
		replyWithVoice = true
		log.Printf("Transcribing %d's audio message", b.chatID)
		b.EditMessageText("Transcribing message...", initialMessageID, nil)

		if msg, err = b.transcript(update.Message.Voice.FileID); err != nil {
			log.Printf("Error transcribing message from user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
			b.EditMessageText("Error transcribing message:\n"+err.Error(), initialMessageID, nil)
			return
		}
	}

	log.Printf("Sending %d's message for conversation %s to ChatGPT", b.chatID, user.SelectedConversation)
	b.EditMessageText("Sending message to ChatGPT...", initialMessageID, nil)

	selectedConversation.AppendMessage(msg, selectedConversation.UserRole)

	if selectedConversation.Title == "" {
		selectedConversation.Title, _ = gpt.SendMessagesToChatGPT(gpt.GetTitle(gpt.DefaultGptEngine, msg))
	}

	response, err := gpt.SendMessagesToChatGPT(selectedConversation)

	if err != nil {
		log.Printf("Error contacting ChatGPT from user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
		b.EditMessageText("error contacting ChatGPT:\n%s"+err.Error(), initialMessageID, nil)

	} else {
		log.Printf("Sending response to user %d for conversation %s", b.chatID, user.SelectedConversation)
		b.EditMessageText("Elaborating response...", initialMessageID, nil)

		if replyWithVoice {
			b.EditMessageText("Obtaining audio...", initialMessageID, nil)

			audioLocation, ttsErr := elevenlabs.TextToSpeech(response)
			if ttsErr != nil {
				log.Printf("Error generating speech from text for user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, ttsErr)
				b.EditMessageText(response+"\n\n"+"Error generating speech from text:\n"+ttsErr.Error(), initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})

			} else {
				log.Printf("Sending audio response for user %d for conversation %s", b.chatID, user.SelectedConversation)

				_, err = b.SendVoice(echotron.NewInputFilePath(audioLocation), b.chatID, &echotron.VoiceOptions{ParseMode: echotron.Markdown, Caption: response})
				if err != nil {
					log.Printf("Error sending audio response for user %d for conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
					b.EditMessageText(response+"\n\n"+"Error sending audio response:\n%s"+err.Error(), initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})

				} else {
					b.DeleteMessage(b.chatID, initialMessage.Result.ID)
				}
			}
		} else {
			b.EditMessageText(response, initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
		}
		selectedConversation.AppendMessage(response, selectedConversation.AssistantRole)

	}
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

func (b *bot) saveUsers(filePath string) error {

	jsonData, err := json.MarshalIndent(b.Users, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal users map: %w", err)
	}

	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (b *bot) loadUsers(filePath string) error {

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	err = json.Unmarshal(jsonData, &b.Users)
	if err != nil {
		return fmt.Errorf("failed to unmarshal b.Users: %w", err)
	}

	return nil
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
		log.Printf("Failed to send notify admin for user %d: %v", chatID, err)
	}
}

func (b *bot) transcript(fileID string) (string, error) {
	var path = ""
	log.Printf("Transcribing message received from: " + strconv.FormatInt(b.chatID, 10))

	if fileMetadata, err := b.GetFile(fileID); err != nil {
		return "", fmt.Errorf("error getting file Metadata: %s", err)
	} else if file, err := b.DownloadFile(fileMetadata.Result.FilePath); err != nil {
		return "", fmt.Errorf("error downloading audio from telegram: %s", err)
	} else if path, err = utils.SaveToTempFile(file, "transcript_"+strconv.FormatInt(b.chatID, 10)+"_*.ogg"); err != nil {
		return "", fmt.Errorf("error saving file to temp folder: %s", err)
	}

	log.Println(path)

	transcript, err := gpt.SendVoiceToWhisper(path)
	if err != nil {
		return "", fmt.Errorf("error sending voice to Whisper: %s", err)
	}
	return transcript, nil
}

func (b *bot) getMainMenu() *echotron.ReplyKeyboardMarkup {
	return &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/list", RequestContact: false, RequestLocation: false},
				{Text: "/new", RequestContact: false, RequestLocation: false},
			},
			{
				{Text: "/settings", RequestContact: false, RequestLocation: false},
				{Text: "/stats", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}
}

func (b *bot) getListOfChats() *echotron.ReplyKeyboardMarkup {
	menu := &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/back", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}

	for _, conv := range b.Users[b.chatID].Conversations {
		command := "/select "
		if conv.Title == "" {
			command += conv.ID.String()
		} else {
			command += conv.Title + " " + conv.ID.String()
		}
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: command}})
	}

	return menu
}

func (b *bot) getConversationUI() *echotron.ReplyKeyboardMarkup {
	menu := &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/back", RequestContact: false, RequestLocation: false},
				{Text: "/stats", RequestContact: false, RequestLocation: false},
			},
			{
				{Text: "/summarize", RequestContact: false, RequestLocation: false},
				{Text: "/delete", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}

	return menu
}

func (b *bot) getPersonalityList() *echotron.ReplyKeyboardMarkup {
	menu := &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/back", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}

	for key, _ := range gpt.Personalities {
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: "/ask " + key}})
	}

	return menu
}
