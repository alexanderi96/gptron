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
		// {Command: "/start", Description: "start bot"},
		// {Command: "/list", Description: "show all conversations"},
		// {Command: "/select", Description: "select conversation"},
		// {Command: "/ping", Description: "check bot status"},
		// {Command: "/help", Description: "help"},
		// {Command: "/whitelist", Description: "whitelist user"},
		// {Command: "/blacklist", Description: "blacklist user"},
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

func (b *bot) handleNewUser() *session.User {
	b.Users[b.chatID] = session.NewUser(strconv.Itoa(int(b.chatID)) == admin, b.chatID)

	if !b.Users[b.chatID].IsAdmin() {
		b.notifyAdmin(b.chatID)
		b.SendMessage("Your request to be whitelisted has been received, please wait for an admin to review it", b.chatID, nil)
	} else {
		b.SendMessage("Welcome back master", b.chatID, nil)
	}
	return b.Users[b.chatID]
}

func (b *bot) handleUserApproval(msg string, user *session.User) {
	if !user.IsAdmin() {
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
	_, markup := b.Users[b.chatID].GetMenu()

	log.Printf("User %d selected conversation %s", b.chatID, argID.String())
	b.SendMessage("Switched to conversation "+argID.String(), b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

	b.Users[b.chatID].SelectedConversation = argID
}
func (b *bot) Update(update *echotron.Update) {
	baseLogCommand := "New %s command from " + strconv.FormatInt(b.chatID, 10)

	msg := message(update)

	user, exists := b.Users[b.chatID]
	if !exists {
		user = b.handleNewUser()
	}

	if user.Status == session.Unreviewed {
		log.Println(baseLogCommand + ". (Not reviewed)")
		b.SendMessage("👀", b.chatID, nil)
		return
	} else if user.Status == session.Blacklisted {
		log.Println(baseLogCommand + ". (Blacklisted)")
		b.SendMessage("💀", b.chatID, nil)
		return
	} else if user.MenuState == "" {
		user.MenuState = session.MenuStateMain
	}

	switch {
	case strings.HasPrefix(msg, "/ping"):
		log.Printf(baseLogCommand, msg)
		b.SendMessage("pong", b.chatID, nil)
		return

	case strings.HasPrefix(msg, "/whitelist"), strings.HasPrefix(msg, "/blacklist"):
		log.Printf(baseLogCommand, msg)
		b.handleUserApproval(msg, user)
		return

	case strings.HasPrefix(msg, "/list"):
		log.Printf(baseLogCommand, msg)
		if len(user.Conversations) <= 0 {
			b.SendMessage("No conversations found, start a new one", b.chatID, nil)
			return
		}

		user.MenuState = session.MenuStateList
		_, markup := user.GetMenu()
		b.SendMessage(
			"Select a conversation from the list or start a new one",
			b.chatID,
			&echotron.MessageOptions{
				ReplyMarkup: markup,
				ParseMode:   echotron.Markdown,
			},
		)
		return

	case strings.HasPrefix(msg, "/select"):
		log.Printf(baseLogCommand, msg)
		if user.HasReachedUsageLimit() {
			b.SendMessage("You have reached your usage limit", b.chatID, nil)
			return
		}
		b.handleSelect(msg)
		return

	case strings.HasPrefix(msg, "/back"):
		log.Printf(baseLogCommand, msg)
		if user.MenuState == session.MenuStateSelected {
			user.MenuState = session.MenuStateList
		} else {
			user.MenuState = session.MenuStateMain
		}
		message, markup := user.GetMenu()

		b.SendMessage(message, b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

		return

	case strings.HasPrefix(msg, "/home"):
		log.Printf(baseLogCommand, msg)
		user.MenuState = session.MenuStateMain
		message, markup := user.GetMenu()
		b.SendMessage(message, b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
		return

	case strings.HasPrefix(msg, "/new"):
		log.Printf(baseLogCommand, msg)
		if user.HasReachedUsageLimit() {
			b.SendMessage("You have reached your usage limit", b.chatID, nil)
			return
		}
		user.SelectedConversation = user.NewConversation()
		user.MenuState = session.MenuStateSelected

	case strings.HasPrefix(msg, "/stats"):
		log.Printf(baseLogCommand, msg)
		if user.MenuState == session.MenuStateMain {
			b.SendMessage(user.GetGlobalStats(), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		} else if user.MenuState == session.MenuStateSelected {
			b.SendMessage(user.GetConversationStats(user.SelectedConversation), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}
		return

	case strings.HasPrefix(msg, "/summarize"):
		log.Printf(baseLogCommand, msg)
		if user.MenuState == session.MenuStateSelected {
			rsp, err := user.Conversations[user.SelectedConversation].Summarize(10)
			if err != nil {
				b.SendMessage(err.Error(), b.chatID, nil)
			} else {
				b.SendMessage(rsp, b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
			}
		}
		return

	case strings.HasPrefix(msg, "/ask"):
		log.Printf(baseLogCommand, msg)
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

				user.MenuState = session.MenuStateSelected
				_, markup := user.GetMenu()

				b.SendMessage("Selected personality "+slice[1]+"\nYou may now start talking with ChatGPT.", b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
				return
			}

		}

	case strings.HasPrefix(msg, "/model"):
		log.Printf(baseLogCommand, msg)
		if user.MenuState == session.MenuStateSelectModel {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				log.Printf("User %d sent an invalid input: %s", b.chatID, msg)
				b.SendMessage("Invalid input", b.chatID, nil)
				return
			}

			for _, model := range gpt.Models {
				if model.Name == slice[1] {
					user.Conversations[user.SelectedConversation].Model = gpt.Models[slice[1]]
					log.Printf("User %d selected model %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].Model.Name, user.SelectedConversation)

					user.MenuState = session.MenuStateSelected
					_, markup := user.GetMenu()

					b.SendMessage("Selected model "+slice[1], b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

				}
			}
			log.Printf("User %d asked a not existing model: %s", b.chatID, msg)
		}

	case strings.HasPrefix(msg, "/delete"):
		log.Printf(baseLogCommand, msg)
		if user.MenuState == session.MenuStateSelected && user.SelectedConversation != uuid.Nil {
			convID := user.SelectedConversation

			if user.Conversations[convID] == nil {
				b.SendMessage("Conversation "+convID.String()+" not found", b.chatID, nil)
				return
			}

			user.Conversations[convID].Delete()

			user.MenuState = session.MenuStateMain
			_, markup := user.GetMenu()

			log.Printf("User %d deleted conversation: %s", b.chatID, convID)
			b.SendMessage("Conversation "+convID.String()+" has been deleted\nGoing back to the Main Menu", b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

		}

	case strings.HasPrefix(msg, "/users_list"):
		if user.IsAdmin() {
			b.SendMessage(b.getUsersList(), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}

	case strings.HasPrefix(msg, "/global_stats"):
		if user.IsAdmin() {
			b.SendMessage(b.getGlobalStats(), b.chatID, &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}

	default:
		if user.MenuState != session.MenuStateSelected {
			_, markup := user.GetMenu()
			b.SendMessage("Select an action from the available ones", b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		}

	}

	if user.MenuState == session.MenuStateSelected {
		if user.Conversations[user.SelectedConversation].Model.Name == "" {
			user.MenuState = session.MenuStateSelectModel
			message, markup := user.GetMenu()
			b.SendMessage(message, b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		} else if user.Conversations[user.SelectedConversation].GptPersonality == "" {
			user.MenuState = session.MenuStateSelectPersonality
			message, markup := user.GetMenu()
			b.SendMessage(message, b.chatID, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		}

		if msg != "" {
			log.Println("User " + strconv.FormatInt(b.chatID, 10) + " talking in conversation " + user.SelectedConversation.String())
			b.handleCommunication(user, msg, update)
			b.Users[b.chatID] = user

		}
	}

	log.Println("Saving users")
	b.saveUsers("users.json")

}

func (b *bot) getUsersList() string {
	if b.Users == nil {
		return "No users found"
	}
	report := "Users list:\n\n"

	for _, u := range b.Users {

		tokens := u.GetTotalTokens()
		cost := u.GetTotalCost()

		report += fmt.Sprintf("User: %d, Status:%s\nTokens: %d\n Cost: $%f\n\n", u.ChatID, u.Status, tokens.PromptTokens+tokens.CompletionTokens, cost.Completion+cost.Prompt)
	}

	return report
}

func (b *bot) getGlobalStats() string {
	if b.Users == nil {
		return "No users found"
	}

	totalInputTokens := 0
	totalOutputTokens := 0

	totalUnreviewedUsers := 0
	totalWhitelistedUsers := 0
	totalBlacklistedUsers := 0
	totalAdmins := 0

	totalInputCosts := float32(0.0)
	totalOutputCosts := float32(0.0)

	report := "Global stats:\n\n"

	report += fmt.Sprintf("Total users: %d\n", len(b.Users))

	for _, u := range b.Users {
		if u.Status == session.Unreviewed {
			totalUnreviewedUsers++
		} else if u.Status == session.Whitelisted {
			totalWhitelistedUsers++
		} else if u.Status == session.Blacklisted {
			totalBlacklistedUsers++
		} else if u.IsAdmin() {
			totalAdmins++
		}

		tokens := u.GetTotalTokens()
		cost := u.GetTotalCost()

		totalInputTokens += tokens.PromptTokens
		totalOutputTokens += tokens.CompletionTokens
		totalInputCosts += cost.Prompt
		totalOutputCosts += cost.Completion
	}

	report += fmt.Sprintf("Total input tokens: %d\n", totalInputTokens)
	report += fmt.Sprintf("Total output tokens: %d\n", totalOutputTokens)
	report += fmt.Sprintf("Total tokens: %d\n", totalInputTokens+totalOutputTokens)
	report += "\n"

	report += fmt.Sprintf("Total input costs: $%f\n", totalInputCosts)
	report += fmt.Sprintf("Total output costs: $%f\n", totalOutputCosts)
	report += fmt.Sprintf("Total costs: $%f\n", totalInputCosts+totalOutputCosts)
	report += "\n"

	return report
}

func (b *bot) handleCommunication(user *session.User, msg string, update *echotron.Update) {

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

	response, err := user.SendMessagesToChatGPT(msg)

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
