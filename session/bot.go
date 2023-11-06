package session

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/NicoNex/echotron/v3"
	"github.com/alexanderi96/gptron/elevenlabs"
	"github.com/alexanderi96/gptron/gpt"
	"github.com/alexanderi96/gptron/utils"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

const filePath = "users.json"

var (
	//go:embed telegram_token
	telegramToken string

	//go:embed admin
	admin string

	adminID int64

	err error

	Dsp *echotron.Dispatcher

	commands = []echotron.BotCommand{
		// {Command: "/start", Description: "start bot"},
		// {Command: "/list", Description: "show all conversations"},
		// {Command: "/select", Description: "select conversation"},
		// {Command: "/ping", Description: "check bot status"},
		// {Command: "/help", Description: "help"},
		// {Command: "/whitelist", Description: "whitelist user"},
		// {Command: "/blacklist", Description: "blacklist user"},
	}
)

type Bot struct {
	chatID int64
	echotron.API
	Users          map[int64]*User
	loggingChannel chan string
}

func init() {
	if len(telegramToken) == 0 {
		log.Fatal("telegram_token not set")
	}

	if len(admin) == 0 {
		log.Fatal("admin not set")
	} else {
		adminID, err = strconv.ParseInt(admin, 10, 64)
		if err != nil {
			log.Fatalf("Failed to parse admin ID: %v", err)
		}
	}

	Dsp = echotron.NewDispatcher(telegramToken, NewBot)
	go setCommands()

}

func setCommands() {
	api := echotron.NewAPI(telegramToken)
	api.SetMyCommands(nil, commands...)
}

func NewBot(chatID int64) echotron.Bot {
	b := &Bot{
		chatID,
		echotron.NewAPI(telegramToken),
		make(map[int64]*User),
		make(chan string, 1),
	}

	go b.startLoggingChannel()

	err := b.LoadUsers()
	if err != nil {
		log.Fatalf("Failed to load user list: %v", err)
	}

	//go b.selfDestruct(time.After(time.Hour))
	return b
}

func (b *Bot) startLoggingChannel() {
	log.Println("Starting logging channel")
	baseMessage := "LOGGING CHANNEL: "
	defer close(b.loggingChannel) // Chiude il canale quando la funzione termina

	for str := range b.loggingChannel {
		// Aggiorna lo stato del messaggio o fai qualcos'altro con l'aggiornamento
		log.Println(baseMessage + str) // Puoi stampare l'aggiornamento o gestirlo in modo appropriato

	}
}

func (b *Bot) Update(update *echotron.Update) {

	baseLogCommand := "New %s command from " + strconv.FormatInt(b.chatID, 10)

	msg := message(update)

	user, exists := b.Users[b.chatID]
	if !exists {
		user = b.handleNewUser()
		b.Users[b.chatID] = user

		if err := b.saveUsers(); err != nil {
			b.sendMessageToUserChan(fmt.Sprintf("Failed to save users: %v", err), &echotron.MessageOptions{
				ParseMode: echotron.Markdown,
			})

		}
	}

	if user.Status == Unreviewed {
		b.loggingChannel <- baseLogCommand + ". (Not reviewed)"
		b.sendMessageToUserChan("👀", &echotron.MessageOptions{})
		return
	} else if user.Status == Blacklisted {
		b.loggingChannel <- baseLogCommand + ". (Blacklisted)"
		b.sendMessageToUserChan("💀", &echotron.MessageOptions{})
		return
	} else if user.MenuState == "" {
		user.MenuState = MenuStateMain
	}

	switch {
	case strings.HasPrefix(msg, "/ping"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		b.sendMessageToUserChan("pong", &echotron.MessageOptions{})
		return

	case strings.HasPrefix(msg, "/whitelist"), strings.HasPrefix(msg, "/blacklist"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		b.handleUserApproval(msg, user)
		return

	case strings.HasPrefix(msg, "/list"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if len(user.Conversations) <= 0 {
			b.sendMessageToUserChan("No conversations found, start a new one", &echotron.MessageOptions{})
			return
		}

		user.MenuState = MenuStateList
		_, markup := user.GetMenu()
		b.sendMessageToUserChan(
			"Select a conversation from the list or start a new one",
			&echotron.MessageOptions{
				ReplyMarkup: markup,
				ParseMode:   echotron.Markdown,
			},
		)
		return

	case strings.HasPrefix(msg, "/select"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.HasReachedUsageLimit() {
			b.sendMessageToUserChan("You have reached your usage limit", &echotron.MessageOptions{})
			return
		}
		b.handleSelect(msg)
		return

	case strings.HasPrefix(msg, "/back"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected {
			user.MenuState = MenuStateList
		} else {
			user.MenuState = MenuStateMain
		}
		message, markup := user.GetMenu()

		b.sendMessageToUserChan(message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

		return

	case strings.HasPrefix(msg, "/home"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		user.MenuState = MenuStateMain
		message, markup := user.GetMenu()
		b.sendMessageToUserChan(message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
		return

	case strings.HasPrefix(msg, "/new"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.HasReachedUsageLimit() {
			b.sendMessageToUserChan("You have reached your usage limit", &echotron.MessageOptions{})
			return
		}
		user.SelectedConversation = user.NewConversation()
		user.MenuState = MenuStateSelected

	case strings.HasPrefix(msg, "/stats"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateMain {
			b.sendMessageToUserChan(user.GetGlobalStats(), &echotron.MessageOptions{ParseMode: echotron.Markdown})
		} else if user.MenuState == MenuStateSelected {
			b.sendMessageToUserChan(user.GetConversationStats(user.SelectedConversation), &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}
		return

	case strings.HasPrefix(msg, "/summarize"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected {
			rsp, err := user.Conversations[user.SelectedConversation].Summarize(10)
			if err != nil {
				b.sendMessageToUserChan(err.Error(), &echotron.MessageOptions{})
			} else {
				b.sendMessageToUserChan(rsp, &echotron.MessageOptions{ParseMode: echotron.Markdown})
			}
		}
		return

	case strings.HasPrefix(msg, "/ask"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelectPersonality {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				log.Printf("User %d sent an invalid input: %s", b.chatID, msg)
				b.sendMessageToUserChan("Invalid input", &echotron.MessageOptions{})
				return
			}

			if gpt.Personalities[slice[1]] == "" {
				log.Printf("User %d asked for personality %s but it does not exist: %s", b.chatID, slice[1], msg)
				b.sendMessageToUserChan("Personality "+slice[1]+" not found", &echotron.MessageOptions{})
				return
			} else {
				user.Conversations[user.SelectedConversation].GptPersonality = slice[1]
				pers, _ := gpt.GetPersonalityWithCommonPrompts(slice[1])
				user.Conversations[user.SelectedConversation].AppendMessage(pers, openai.ChatMessageRoleSystem)
				log.Printf("User %d selected personality %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].GptPersonality, user.SelectedConversation)

				user.MenuState = MenuStateSelected
				_, markup := user.GetMenu()

				b.sendMessageToUserChan("Selected personality "+slice[1]+"\nYou may now start talking with ChatGPT.", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
				return
			}

		}

	case strings.HasPrefix(msg, "/model"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelectModel {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				log.Printf("User %d sent an invalid input: %s", b.chatID, msg)
				b.sendMessageToUserChan("Invalid input", &echotron.MessageOptions{})
				return
			}

			for _, model := range gpt.Models {
				if model.Name == slice[1] {
					user.Conversations[user.SelectedConversation].Model = gpt.Models[slice[1]]
					log.Printf("User %d selected model %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].Model.Name, user.SelectedConversation)

					user.MenuState = MenuStateSelected
					_, markup := user.GetMenu()

					b.sendMessageToUserChan("Selected model "+slice[1], &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

				}
			}
			log.Printf("User %d asked a not existing model: %s", b.chatID, msg)
		}

	case strings.HasPrefix(msg, "/delete"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected && user.SelectedConversation != uuid.Nil {
			convID := user.SelectedConversation

			if user.Conversations[convID] == nil {
				b.sendMessageToUserChan("Conversation "+convID.String()+" not found", &echotron.MessageOptions{})
				return
			}

			user.Conversations[convID].Delete()

			user.MenuState = MenuStateMain
			_, markup := user.GetMenu()

			log.Printf("User %d deleted conversation: %s", b.chatID, convID)
			b.sendMessageToUserChan("Conversation "+convID.String()+" has been deleted\nGoing back to the Main Menu", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

		}

	case strings.HasPrefix(msg, "/users_list"):
		if user.IsAdmin() {
			b.sendMessageToUserChan(b.getUsersList(), &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}

	case strings.HasPrefix(msg, "/global_stats"):
		if user.IsAdmin() {
			b.sendMessageToUserChan(b.getGlobalStats(), &echotron.MessageOptions{ParseMode: echotron.Markdown})
		}

	default:
		if user.MenuState != MenuStateSelected {
			_, markup := user.GetMenu()
			b.sendMessageToUserChan("Select an action from the available ones", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		}

	}

	if user.MenuState == MenuStateSelected {
		if user.Conversations[user.SelectedConversation].Model.Name == "" {
			user.MenuState = MenuStateSelectModel
			message, markup := user.GetMenu()
			b.sendMessageToUserChan(message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		} else if user.Conversations[user.SelectedConversation].GptPersonality == "" {
			user.MenuState = MenuStateSelectPersonality
			message, markup := user.GetMenu()
			b.sendMessageToUserChan(message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})
			return
		}

		if msg != "" {
			log.Println("User " + strconv.FormatInt(b.chatID, 10) + " talking in conversation " + user.SelectedConversation.String())
			b.handleCommunication(user, msg, update)
			b.Users[b.chatID] = user

		}
	}

	if err := b.saveUsers(); err != nil {
		b.sendMessageToUserChan(fmt.Sprintf("Failed to save users: %v", err), &echotron.MessageOptions{
			ParseMode: echotron.Markdown,
		})
	}

}

func (b *Bot) sendMessageToUserChan(msg string, opt *echotron.MessageOptions) {
	b.Users[b.chatID].messageChan <- &msgCtx{
		bot:         b,
		msg:         msg,
		replyMarkup: opt,
	}
}

func (b *Bot) sendMessageToAdminChan(msg string, opt *echotron.MessageOptions) {
	b.Users[adminID].messageChan <- &msgCtx{
		bot:         b,
		msg:         msg,
		replyMarkup: opt,
	}
}

func (b *Bot) handleNewUser() *User {
	user := NewUser(strconv.Itoa(int(b.chatID)) == admin, b.chatID)

	str := "Welcome new user"

	if !user.IsAdmin() {
		b.loggingChannel <- "New user: " + strconv.Itoa(int(b.chatID))
		b.notifyAdmin()
		str = "Your request to be whitelisted has been received, please wait for an admin to review it"
	} else {
		str = "Welcome back master"
	}
	b.sendMessageToUserChan(str, &echotron.MessageOptions{})

	return user
}

func (b *Bot) handleUserApproval(msg string, user *User) {
	if !user.IsAdmin() {
		b.sendMessageToUserChan("Only admins can use this command", &echotron.MessageOptions{})
		return
	}
	slice := strings.Split(msg, " ")

	if len(slice) != 2 && utils.IsNumber(slice[1]) {
		b.sendMessageToUserChan("Invalid chat ID: "+slice[1], &echotron.MessageOptions{})
		return
	}
	userChatID, _ := strconv.Atoi(slice[1])
	if slice[0] == "/whitelist" {
		if b.Users[int64(userChatID)].Status == Whitelisted {
			b.sendMessageToUserChan("User "+slice[1]+" already whitelisted", &echotron.MessageOptions{})
			return
		}
		b.Users[int64(userChatID)].Status = Whitelisted
		b.sendMessageToUserChan("You have been whitelisted", &echotron.MessageOptions{})
	} else if slice[0] == "/blacklist" {
		if b.Users[int64(userChatID)].Status == Blacklisted {
			b.sendMessageToUserChan("User "+slice[1]+" already blacklisted", &echotron.MessageOptions{})
			return
		}
		b.Users[int64(userChatID)].Status = Blacklisted
		b.sendMessageToUserChan("You have been blacklisted", &echotron.MessageOptions{})
	}
}

func (b *Bot) handleSelect(msg string) {
	slice := strings.Split(msg, " ")

	if len(slice) < 2 && utils.IsUUID(slice[len(slice)-1]) {
		b.sendMessageToUserChan("Invalid chat ID", &echotron.MessageOptions{})
		return
	}

	argID, _ := uuid.Parse(slice[len(slice)-1])

	if b.Users[b.chatID].Conversations[argID] == nil {
		b.sendMessageToUserChan("Conversation "+argID.String()+" not found", &echotron.MessageOptions{})
		return
	}
	b.Users[b.chatID].MenuState = MenuStateSelected
	_, markup := b.Users[b.chatID].GetMenu()

	log.Printf("User %d selected conversation %s", b.chatID, argID.String())
	b.sendMessageToUserChan("Switched to conversation "+argID.String(), &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown})

	b.Users[b.chatID].SelectedConversation = argID
}

func (b *Bot) getUsersList() string {
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

func (b *Bot) getGlobalStats() string {
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
		if u.Status == Unreviewed {
			totalUnreviewedUsers++
		} else if u.Status == Whitelisted {
			totalWhitelistedUsers++
		} else if u.Status == Blacklisted {
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

func (b *Bot) handleCommunication(user *User, msg string, update *echotron.Update) {

	initialMessage, err := b.SendMessage("Analizing message...", b.chatID, &echotron.MessageOptions{})
	if err != nil {
		log.Printf("Error sending initial message to %d: %s", b.chatID, err)
		return
	}

	initialMessageID := echotron.NewMessageID(b.chatID, initialMessage.Result.ID)

	if update.Message != nil && update.Message.Voice != nil {
		var err error
		user.replyWithVoice = true
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

	response, err := user.sendMessagesToChatGPT(msg)

	if err != nil {
		log.Printf("Error contacting ChatGPT from user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
		b.EditMessageText("error contacting ChatGPT:\n%s"+err.Error(), initialMessageID, nil)
	} else {
		log.Printf("Sending response to user %d for conversation %s", b.chatID, user.SelectedConversation)
		b.EditMessageText("Elaborating response...", initialMessageID, nil)

		if user.replyWithVoice {
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

func (b *Bot) notifyAdmin() {

	b.sendMessageToAdminChan(fmt.Sprintf("Nuovo utente non revisionato: %d", b.chatID),
		&echotron.MessageOptions{ReplyMarkup: echotron.InlineKeyboardMarkup{
			InlineKeyboard: [][]echotron.InlineKeyboardButton{
				{
					echotron.InlineKeyboardButton{Text: "Whitelist", CallbackData: fmt.Sprintf("/whitelist %d", b.chatID)},
					echotron.InlineKeyboardButton{Text: "Blacklist", CallbackData: fmt.Sprintf("/blacklist %d", b.chatID)},
				},
			},
		}},
	)
	if err != nil {
		b.loggingChannel <- fmt.Sprintf("Failed to send notify admin for user %d: %v", b.chatID, err)
	}
}

func (b *Bot) transcript(fileID string) (string, error) {
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

func (b *Bot) saveUsers() error {
	b.loggingChannel <- "Saving users..."

	jsonData, err := json.MarshalIndent(b.Users, "", "  ")
	if err != nil {
		b.loggingChannel <- fmt.Sprintf("failed to marshal users map: %w", err)
		return err
	}

	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		b.loggingChannel <- fmt.Sprintf("failed to write file: %w", err)
		return err
	}

	return nil
}

func (b *Bot) LoadUsers() error {
	b.loggingChannel <- "Loading users..."

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

	b.initializeUsers()

	return nil
}

func (b *Bot) initializeUsers() {
	for _, user := range b.Users {
		user.startMessagesChannel()
		b.Users[user.ChatID] = user
	}
}
