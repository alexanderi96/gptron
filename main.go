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

	defaultGptEngine = "gpt-4"
)

func init() {
	if len(telegramToken) == 0 {
		log.Fatal("telegram_token not set")
	}

	if len(admin) == 0 {
		log.Fatal("admin not set")
	}

	dsp = echotron.NewDispatcher(telegramToken, newBot)
	go setCommands()

}

func newBot(chatID int64) echotron.Bot {
	bot := &bot{
		chatID,
		echotron.NewAPI(telegramToken),
		make(map[int64]*session.User),
	}

	err := bot.loadUsers("users.json")
	if err != nil {
		log.Fatalf("Failed to load user list: %x", err)
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

func (b *bot) Update(update *echotron.Update) {
	log.Printf("Message recieved from: " + strconv.FormatInt(b.chatID, 10))

	msg := message(update)
	replyWithVoice := false

	user, exists := b.Users[b.chatID]
	if !exists {

		user = &session.User{
			Status:               session.Unreviewed,
			IsAdmin:              strconv.Itoa(int(b.chatID)) == admin,
			Conversations:        make(map[uuid.UUID]*session.Conversation),
			SelectedConversation: uuid.Nil,
		}

		b.Users[b.chatID] = user
		b.notifyAdmin(b.chatID)

		rpl, err := gpt.SendTextToChatGPT(gpt.MustBeApproved(defaultGptEngine))
		if err != nil {
			log.Println(err)
			b.SendMessage("Your request to be whitelisted has been received, please wait for an admin to review it", b.chatID, nil)
		} else {
			b.SendMessage(rpl, b.chatID, nil)
		}

		return

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

	switch {
	case strings.HasPrefix(msg, "/ping"):
		b.SendMessage("pong", b.chatID, nil)

	case strings.HasPrefix(msg, "/whitelist"):
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

		if b.Users[int64(userChatID)].Status == session.Whitelisted {
			b.SendMessage("User "+slice[1]+" already whitelisted", b.chatID, nil)
			return
		}
		b.Users[int64(userChatID)].Status = session.Whitelisted
		b.SendMessage("You have been whitelisted", int64(userChatID), nil)

		b.SendMessage("User "+slice[1]+" has been whitelisted", b.chatID, nil)

	case strings.HasPrefix(msg, "/blacklist"):
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

		if b.Users[int64(userChatID)].Status == session.Blacklisted {
			b.SendMessage("User "+slice[1]+" already blacklisted", b.chatID, nil)
			return
		}
		b.Users[int64(userChatID)].Status = session.Blacklisted
		b.SendMessage("You have been blacklisted", int64(userChatID), nil)

		b.SendMessage("User "+slice[1]+" has been blacklisted", b.chatID, nil)

	case strings.HasPrefix(msg, "/list"):
		user.MenuState = session.MenuStateList
		b.SendMessage("Select a conversation from the list or start a new one", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getListOfChats(), ParseMode: echotron.Markdown})

	case strings.HasPrefix(msg, "/select"):
		slice := strings.Split(msg, " ")

		if len(slice) != 2 && utils.IsUUID(slice[1]) {
			b.SendMessage("Invalid chat ID", b.chatID, nil)
			return
		}

		chatID, _ := uuid.Parse(slice[1])

		if b.Users[b.chatID].Conversations[chatID] == nil {
			b.SendMessage("Conversation "+slice[1]+" not found", b.chatID, nil)
			return
		}

		log.Println("Selected conversation: ", chatID)
		b.SendMessage("Conversation "+slice[1]+" selected", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getBackButton(), ParseMode: echotron.Markdown})

		b.Users[b.chatID].SelectedConversation = chatID
		b.Users[b.chatID].MenuState = session.MenuStateSelected

	case strings.HasPrefix(msg, "/back"):
		if user.MenuState == session.MenuStateSelected {
			user.MenuState = session.MenuStateList
			b.SendMessage("Select a conversation from the list", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getListOfChats(), ParseMode: echotron.Markdown})
		} else if user.MenuState == session.MenuStateList {
			user.MenuState = session.MenuStateMain
			b.SendMessage("Main menu", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMenu(), ParseMode: echotron.Markdown})
		}
		return

	case strings.HasPrefix(msg, "/new"):
		user.MenuState = session.MenuStateSelected

		user.CreateNewConversation(defaultGptEngine, strconv.Itoa(int(b.chatID)))

		rpl, err := gpt.SendTextToChatGPT(gpt.NewChat(defaultGptEngine))
		if err != nil {
			log.Println(err)
			b.SendMessage("You may now start talking with ChatGPT.", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getBackButton()})
		} else {
			b.SendMessage(rpl, b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getBackButton()})
		}
		return

	default:
		if user.MenuState != session.MenuStateSelected {
			b.SendMessage("Select a conversation from the list or start a new one", b.chatID, &echotron.MessageOptions{ReplyMarkup: b.getMenu(), ParseMode: echotron.Markdown})
			return
		}

		initialMessage, err := b.SendMessage("Analizing message...", b.chatID, nil)
		if err != nil {
			log.Printf("Error sending initial message to %d: %s", b.chatID, err)
			return
		}

		initialMessageID := echotron.NewMessageID(b.chatID, initialMessage.Result.ID)

		if update.Message != nil && update.Message.Voice != nil {
			var err error
			replyWithVoice = true
			log.Printf("Transcribing %d's message", b.chatID)
			b.EditMessageText("Transcribing message...", initialMessageID, nil)

			if msg, err = b.transcript(update.Message.Voice.FileID); err != nil {
				errorMessage := fmt.Errorf("error transcribing message: %s", err)
				log.Println(errorMessage)
				b.EditMessageText(errorMessage.Error(), initialMessageID, nil)
			}
		}

		log.Printf("Sending %d's message to ChatGPT", b.chatID)
		_, err = b.EditMessageText("Sending message to ChatGPT...", initialMessageID, nil)

		if err != nil {
			log.Printf("Error editing message %d: %s", b.chatID, err)
			return
		}
		response, err := gpt.SendMessagesToChatGPT(*user.AppendMessageAndGetConversation(msg), defaultGptEngine)
		if err != nil {
			errorMessage := fmt.Errorf("error contacting ChatGPT: %s", err)
			log.Println(errorMessage)
			b.EditMessageText(errorMessage.Error(), initialMessageID, nil)

		} else {
			log.Printf("Sending response to %d", b.chatID)
			b.EditMessageText("Elaborating response...", initialMessageID, nil)

			if replyWithVoice {
				b.EditMessageText("Obtaining audio...", initialMessageID, nil)

				audioLocation, ttsErr := elevenlabs.TextToSpeech(response)
				if ttsErr != nil {
					errorMessage := fmt.Errorf("error generating speech from text: %s", ttsErr)
					log.Println(errorMessage)
					b.EditMessageText(response+"\n\n"+errorMessage.Error(), initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
				} else {
					log.Printf("Sending audio to %d", b.chatID)

					_, err = b.SendVoice(echotron.NewInputFilePath(audioLocation), b.chatID, &echotron.VoiceOptions{ParseMode: echotron.Markdown, Caption: response})
					if err != nil {
						errorMessage := fmt.Errorf("error sending audio to %d: %s", b.chatID, err)
						log.Println(errorMessage)
						b.EditMessageText(response+"\n\n"+"error sending audio response", initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
					} else {
						b.DeleteMessage(b.chatID, initialMessage.Result.ID)
					}
				}
			} else {
				b.EditMessageText(response, initialMessageID, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
			}
		}
	}
	b.saveUsers("users.json")

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
		log.Printf("Failed to send message to admin: %v", err)
	}
}

func (b *bot) transcript(fileID string) (string, error) {
	log.Printf("Transcribing message received from: " + strconv.FormatInt(b.chatID, 10))

	fileMetadata, err := b.GetFile(fileID)

	if err != nil {
		return "", fmt.Errorf("error getting file Metadata: %s", err)
	}

	transcript, err := gpt.SendVoiceToWhisper(fileMetadata.Result.FilePath)
	if err != nil {
		return "", fmt.Errorf("error sending voice to Whisper: %s", err)
	}
	return transcript, nil
}

func (b *bot) getMenu() *echotron.ReplyKeyboardMarkup {
	return &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/list", RequestContact: false, RequestLocation: false},
				{Text: "/new", RequestContact: false, RequestLocation: false},
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
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: "/select " + conv.ID.String()}})
	}

	return menu
}

func (b *bot) getBackButton() *echotron.ReplyKeyboardMarkup {
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

	return menu
}
