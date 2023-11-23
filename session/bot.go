package session

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NicoNex/echotron/v3"
	"github.com/alexanderi96/gptron/elevenlabs"
	"github.com/alexanderi96/gptron/gpt"
	"github.com/alexanderi96/gptron/utils"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

const appDir = "gptron"
const dataFileName = "users.json"

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

	fullFilePath string
)

type Bot struct {
	chatID int64
	echotron.API
	users          map[int64]*User
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

	homePath, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	fullFilePath = homePath + "/" + appDir + "/" + dataFileName
}

func createDirIfNotExist(filePath string) error {
	directory := filepath.Dir(filePath)
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err := os.MkdirAll(directory, 0755)
		return err
	}

	// Crea il file se non esiste già
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if _, err := os.Create(filePath); err != nil {
			return err
		}
	}
	return nil
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

	//go b.selfDestruct(time.After(time.Hour))
	return b
}

func (b *Bot) startLoggingChannel() {
	log.Println("Starting logging channel")
	baseMessage := ""
	defer close(b.loggingChannel) // Chiude il canale quando la funzione termina

	for str := range b.loggingChannel {
		// Aggiorna lo stato del messaggio o fai qualcos'altro con l'aggiornamento
		log.Println(baseMessage + str) // Puoi stampare l'aggiornamento o gestirlo in modo appropriato

	}
}

func (b *Bot) Update(update *echotron.Update) {
	defer func() {
		if err := b.saveUsers(); err != nil {
			b.sendMessageToUserChan(nil, b.chatID, fmt.Sprintf("Failed to save users: %v", err), &echotron.MessageOptions{
				ParseMode: echotron.Markdown,
			}, nil)
		}
	}()

	err := b.loadUsers()
	if err != nil {
		log.Fatalf("Failed to load user list: %v", err)
	}

	baseLogCommand := "New %s command from " + strconv.FormatInt(b.chatID, 10)

	msg := message(update)

	user, exists := b.users[b.chatID]
	if !exists {
		b.handleNewUser()
		if err := b.saveUsers(); err != nil {
			b.sendMessageToUserChan(nil, b.chatID, fmt.Sprintf("Failed to save users: %v", err), &echotron.MessageOptions{
				ParseMode: echotron.Markdown,
			}, nil)

		}
		return
	}

	if user.Status == Unreviewed {
		b.loggingChannel <- baseLogCommand + ". (Not reviewed)"
		b.sendMessageToUserChan(nil, b.chatID, "👀", nil, nil)
		return
	} else if user.Status == Blacklisted {
		b.loggingChannel <- baseLogCommand + ". (Blacklisted)"
		b.sendMessageToUserChan(nil, b.chatID, "💀", nil, nil)
		return
	} else if user.MenuState == "" {
		user.MenuState = MenuStateMain
	}

	switch {
	case strings.HasPrefix(msg, "/ping"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		b.sendMessageToUserChan(nil, b.chatID, "pong", nil, nil)
		return

	case strings.HasPrefix(msg, "/whitelist"), strings.HasPrefix(msg, "/blacklist"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		b.handleUserApproval(msg)
		return

	case strings.HasPrefix(msg, "/list"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if len(user.Conversations) <= 0 {
			b.sendMessageToUserChan(nil, b.chatID, "No conversations found, start a new one", nil, nil)
			return
		}

		user.MenuState = MenuStateList
		_, markup := user.GetMenu()
		b.sendMessageToUserChan(
			nil,
			b.chatID,
			"Select a conversation from the list or start a new one",
			&echotron.MessageOptions{
				ReplyMarkup: markup,
				ParseMode:   echotron.Markdown,
			},
			nil,
		)
		return

	case strings.HasPrefix(msg, "/select"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
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
		b.sendMessageToUserChan(nil, b.chatID, message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
		return

	case strings.HasPrefix(msg, "/home"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		user.MenuState = MenuStateMain
		message, markup := user.GetMenu()
		b.sendMessageToUserChan(nil, b.chatID, message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
		return

	case strings.HasPrefix(msg, "/new"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		user.MenuState = MenuStateSelected
		user.SelectedConversation = user.NewConversation()

	case strings.HasPrefix(msg, "/stats"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateMain {
			b.sendMessageToUserChan(nil, b.chatID, user.GetStatsForAllChats(), &echotron.MessageOptions{ParseMode: echotron.Markdown}, nil)
		} else if user.MenuState == MenuStateSelected {
			b.sendMessageToUserChan(nil, b.chatID, user.GetConversationStats(user.SelectedConversation), &echotron.MessageOptions{ParseMode: echotron.Markdown}, nil)
		}
		return

	case strings.HasPrefix(msg, "/summarize"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected {
			rsp, err := user.Conversations[user.SelectedConversation].Summarize(10)
			if err != nil {
				b.sendMessageToUserChan(nil, b.chatID, err.Error(), nil, nil)
			} else {
				b.sendMessageToUserChan(nil, b.chatID, rsp, &echotron.MessageOptions{ParseMode: echotron.Markdown}, nil)
			}
		}
		return

	case strings.HasPrefix(msg, "/ask"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelectPersonality {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				b.loggingChannel <- fmt.Sprintf("User %d sent an invalid input: %s", b.chatID, msg)
				b.sendMessageToUserChan(nil, b.chatID, "Invalid input", nil, nil)
				return
			}

			if gpt.Personalities[slice[1]] == "" {
				b.loggingChannel <- fmt.Sprintf("User %d asked for personality %s but it does not exist: %s", b.chatID, slice[1], msg)
				b.sendMessageToUserChan(nil, b.chatID, "Personality "+slice[1]+" not found", nil, nil)
				return
			} else {
				user.Conversations[user.SelectedConversation].GptPersonality = slice[1]
				pers, _ := gpt.GetPersonalityWithCommonPrompts(slice[1])
				user.Conversations[user.SelectedConversation].AppendMessage(pers, openai.ChatMessageRoleSystem)
				b.loggingChannel <- fmt.Sprintf("User %d selected personality %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].GptPersonality, user.SelectedConversation)

				user.MenuState = MenuStateSelected
				_, markup := user.GetMenu()

				b.sendMessageToUserChan(nil, b.chatID, "Selected personality "+slice[1]+"\nYou may now start talking with ChatGPT.", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
			}
		}
		return

	case strings.HasPrefix(msg, "/model"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelectModel {
			slice := strings.Split(msg, " ")

			if len(slice) != 2 {
				b.loggingChannel <- fmt.Sprintf("User %d sent an invalid input: %s", b.chatID, msg)
				b.sendMessageToUserChan(nil, b.chatID, "Invalid input", nil, nil)
				return
			}

			for _, model := range gpt.Models {
				log.Println(model.Name)
				if model.Name == slice[1] {
					if model.Restricted && !user.IsAdmin() {
						continue
					}
					user.Conversations[user.SelectedConversation].Model = gpt.Models[slice[1]]
					b.loggingChannel <- fmt.Sprintf("User %d selected model %s for conversation %s", b.chatID, user.Conversations[user.SelectedConversation].Model.Name, user.SelectedConversation)

					user.MenuState = MenuStateSelected
					_, markup := user.GetMenu()

					b.sendMessageToUserChan(nil, b.chatID, "Selected model "+slice[1], &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
				}
			}
			if user.MenuState != MenuStateSelected {
				b.loggingChannel <- fmt.Sprintf("User %d asked a not existing model: %s", b.chatID, msg)
				b.sendMessageToUserChan(nil, b.chatID, "I'm afraid the model "+slice[1]+" is not available", nil, nil)
			}
		}

	case strings.HasPrefix(msg, "/delete"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected && user.SelectedConversation != uuid.Nil {
			convID := user.SelectedConversation

			if user.Conversations[convID] == nil {
				b.sendMessageToUserChan(nil, b.chatID, "Conversation "+convID.String()+" not found", nil, nil)
				return
			}

			b.SendDocument(echotron.NewInputFileBytes(convID.String()+"_summary.md", []byte(user.Conversations[convID].GenerateReport())), user.ChatID, &echotron.DocumentOptions{Caption: "Summary of conversation " + convID.String()})
			user.Conversations[convID].Delete()

			user.MenuState = MenuStateMain
			_, markup := user.GetMenu()

			b.loggingChannel <- fmt.Sprintf("User %d deleted conversation: %s", b.chatID, convID)
			b.sendMessageToUserChan(nil, b.chatID, "Conversation "+convID.String()+" has been deleted\nGoing back to the Main Menu", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)

		}
		return

	case strings.HasPrefix(msg, "/generate_report"):
		b.loggingChannel <- fmt.Sprintf(baseLogCommand, msg)
		if user.MenuState == MenuStateSelected && user.SelectedConversation != uuid.Nil {
			convID := user.SelectedConversation

			if user.Conversations[convID] == nil {
				b.sendMessageToUserChan(nil, b.chatID, "Conversation "+convID.String()+" not found", nil, nil)
				return
			}

			b.SendDocument(echotron.NewInputFileBytes(convID.String()+"_summary.md", []byte(user.Conversations[convID].GenerateReport())), user.ChatID, &echotron.DocumentOptions{Caption: "Summary of conversation " + convID.String()})
		}
		return

	case strings.HasPrefix(msg, "/users_list"):
		if user.IsAdmin() {
			b.sendMessageToUserChan(nil, b.chatID, b.getUsersList(), &echotron.MessageOptions{ParseMode: echotron.Markdown}, nil)
		}
		return

	case strings.HasPrefix(msg, "/global_stats"):
		if user.IsAdmin() {
			b.sendMessageToUserChan(nil, b.chatID, b.getAdminStats(), &echotron.MessageOptions{ParseMode: echotron.Markdown}, nil)
		}
		return

	default:
		if user.MenuState != MenuStateSelected {
			_, markup := user.GetMenu()
			b.sendMessageToUserChan(nil, b.chatID, "Select an action from the available ones", &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
			return
		}

	}

	if user.MenuState == MenuStateSelected {
		if user.Conversations[user.SelectedConversation].Model.Name == "" {
			user.MenuState = MenuStateSelectModel
			message, markup := user.GetMenu()
			b.sendMessageToUserChan(nil, b.chatID, message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
			return
		} else if user.Conversations[user.SelectedConversation].GptPersonality == "" {
			user.MenuState = MenuStateSelectPersonality
			message, markup := user.GetMenu()
			b.sendMessageToUserChan(nil, b.chatID, message, &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)
			return
		}
		if user.HasReachedUsageLimit() && !user.IsAdmin() {
			b.sendMessageToUserChan(nil, b.chatID, "I'm sorry Dave, I'm afraid I can't do that.\n(You have reached your usage limit)", nil, nil)
			return
		} else {
			log.Println("User " + strconv.FormatInt(b.chatID, 10) + " talking in conversation " + user.SelectedConversation.String())
			b.handleCommunication(user, msg, update)
			b.users[b.chatID] = user
		}
	}
}

func (b *Bot) sendMessageToUserChan(lastSentMessageIDOption *echotron.MessageIDOptions, chatID int64, msg string, msgOpt *echotron.MessageOptions, msgTextOpt *echotron.MessageTextOptions) {
	b.users[b.chatID].messageChan <- &msgCtx{
		initialMessageID:   lastSentMessageIDOption,
		bot:                b,
		chatID:             chatID,
		msg:                msg,
		messageOptions:     msgOpt,
		messageTextOptions: msgTextOpt,
	}
}

func (b *Bot) sendMessageToAdminChan(lastSentMessageIDOption *echotron.MessageIDOptions, msg string, msgOpt *echotron.MessageOptions, msgTextOpt *echotron.MessageTextOptions) {
	b.users[adminID].messageChan <- &msgCtx{
		initialMessageID:   lastSentMessageIDOption,
		bot:                b,
		chatID:             adminID,
		msg:                msg,
		messageOptions:     msgOpt,
		messageTextOptions: msgTextOpt,
	}
}

func (b *Bot) handleNewUser() {
	b.loggingChannel <- "New user: " + strconv.Itoa(int(b.chatID))
	user := newUser(b.chatID)

	str := "Welcome new user"

	if !user.IsAdmin() {
		b.loggingChannel <- "New user: " + strconv.Itoa(int(b.chatID))
		b.notifyAdmin()
		str = "Your request to be whitelisted has been received, please wait for an admin to review it"
	} else {
		b.loggingChannel <- "New admin: " + strconv.Itoa(int(b.chatID))
		str = "Welcome back master"
	}

	b.users[b.chatID] = user

	b.sendMessageToUserChan(nil, b.chatID, str, nil, nil)

}

func (b *Bot) handleUserApproval(msg string) {
	if !b.users[b.chatID].IsAdmin() {
		b.sendMessageToUserChan(nil, b.chatID, "Only admins can use this command", nil, nil)
		return
	}

	slice := strings.Split(msg, " ")

	if len(slice) != 2 && utils.IsNumber(slice[1]) {
		b.sendMessageToUserChan(nil, b.chatID, "Invalid chat ID: "+slice[1], nil, nil)
		return
	}
	userChatID, _ := strconv.Atoi(slice[1])
	int64ChatId := int64(userChatID)

	if slice[0] == "/whitelist" {
		if b.users[int64ChatId].Status == Whitelisted {
			b.sendMessageToUserChan(nil, b.chatID, "User "+slice[1]+" already whitelisted", nil, nil)
			return
		}
		b.users[int64ChatId].Status = Whitelisted
		b.sendMessageToUserChan(nil, int64ChatId, "You have been whitelisted", nil, nil)
	} else if slice[0] == "/blacklist" {
		if b.users[int64ChatId].Status == Blacklisted {
			b.sendMessageToUserChan(nil, b.chatID, "User "+slice[1]+" already blacklisted", nil, nil)
			return
		}
		b.users[int64ChatId].Status = Blacklisted
		b.sendMessageToUserChan(nil, int64ChatId, "You have been blacklisted", nil, nil)
	}
}

func (b *Bot) handleSelect(msg string) {
	slice := strings.Split(msg, " ")

	if len(slice) < 2 && utils.IsUUID(slice[len(slice)-1]) {
		b.sendMessageToUserChan(nil, b.chatID, "Invalid chat ID", nil, nil)
		return
	}

	argID, _ := uuid.Parse(slice[len(slice)-1])

	if b.users[b.chatID].Conversations[argID] == nil {
		b.sendMessageToUserChan(nil, b.chatID, "Conversation "+argID.String()+" not found", nil, nil)
		return
	}
	b.users[b.chatID].MenuState = MenuStateSelected
	_, markup := b.users[b.chatID].GetMenu()

	b.loggingChannel <- fmt.Sprintf("User %d selected conversation %s", b.chatID, argID.String())
	b.sendMessageToUserChan(nil, b.chatID, "Switched to conversation "+argID.String(), &echotron.MessageOptions{ReplyMarkup: markup, ParseMode: echotron.Markdown}, nil)

	b.users[b.chatID].SelectedConversation = argID
}

func (b *Bot) getUsersList() string {
	if b.users == nil {
		return "No users found"
	}
	report := "Users list:\n\n"

	for _, u := range b.users {

		tokens := u.GetTotalTokens()
		cost := u.GetTotalCost()

		report += fmt.Sprintf("User: %d, Status:%s\nTokens: %d\n Cost: $%f\n\n", u.ChatID, u.Status, tokens.PromptTokens+tokens.CompletionTokens, cost.Completion+cost.Prompt)
	}

	return report
}

func (b *Bot) getAdminStats() string {
	if b.users == nil {
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

	report := "Admin stats:\n\n"

	report += fmt.Sprintf("Total users: %d\n\n", len(b.users))

	for _, u := range b.users {
		tokens := u.GetTotalTokens()
		cost := u.GetTotalCost()

		totalInputTokens += tokens.PromptTokens
		totalOutputTokens += tokens.CompletionTokens
		totalInputCosts += cost.Prompt
		totalOutputCosts += cost.Completion

		if u.IsAdmin() {
			totalAdmins++
		} else {
			if u.Status == Unreviewed {
				totalUnreviewedUsers++
			} else if u.Status == Whitelisted {
				totalWhitelistedUsers++
			} else if u.Status == Blacklisted {
				totalBlacklistedUsers++
			}
		}
	}

	report += fmt.Sprintf("Total input tokens: %d ($%f)\n", totalInputTokens, totalInputCosts)
	report += fmt.Sprintf("Total output tokens: %d ($%f)\n", totalOutputTokens, totalOutputCosts)
	report += fmt.Sprintf("Total tokens: %d ($%f)\n", totalInputTokens+totalOutputTokens, totalInputCosts+totalOutputCosts)
	report += "\n"

	return report
}

func (b *Bot) handleCommunication(user *User, msg string, update *echotron.Update) {

	b.sendMessageToUserChan(nil, b.chatID, "Analizing message...", nil, nil)

	if err != nil {
		b.loggingChannel <- fmt.Sprintf("Error sending initial message to %d: %s", b.chatID, err)
		return
	}

	if update.Message != nil && update.Message.Voice != nil {
		var err error
		user.replyWithVoice = true
		b.loggingChannel <- fmt.Sprintf("Transcribing %d's audio message", b.chatID)

		b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Transcribing message...", nil, nil)

		if msg, err = b.transcript(update.Message.Voice.FileID); err != nil {
			b.loggingChannel <- fmt.Sprintf("Error transcribing message from user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
			b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Error transcribing message:\n"+err.Error(), nil, nil)
			return
		}
	}

	b.loggingChannel <- fmt.Sprintf("Sending %d's message for conversation %s to ChatGPT", b.chatID, user.SelectedConversation)
	b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Sending message to ChatGPT...", nil, nil)

	response, err := user.sendMessagesToChatGPT(msg, b)

	if err != nil {
		b.loggingChannel <- fmt.Sprintf("Error contacting ChatGPT from user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
		b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Error contacting ChatGPT:\n%s"+err.Error(), nil, nil)

	} else {
		b.loggingChannel <- fmt.Sprintf("Sending response to user %d for conversation %s", b.chatID, user.SelectedConversation)
		b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Elaborating response...", nil, nil)

		if user.replyWithVoice {
			b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, "Obtaining audio...", nil, nil)

			audioLocation, ttsErr := elevenlabs.TextToSpeech(response)
			if ttsErr != nil {
				b.loggingChannel <- fmt.Sprintf("Error generating speech from text for user %d at conversation %s:\n%s", b.chatID, user.SelectedConversation, ttsErr)
				b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, response+"\n\n"+"Error generating speech from text:\n"+ttsErr.Error(), nil, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})

			} else {
				b.loggingChannel <- fmt.Sprintf("Sending audio response for user %d for conversation %s", b.chatID, user.SelectedConversation)

				_, err = b.SendVoice(echotron.NewInputFilePath(audioLocation), b.chatID, &echotron.VoiceOptions{ParseMode: echotron.Markdown, Caption: response})
				if err != nil {
					b.loggingChannel <- fmt.Sprintf("Error sending audio response for user %d for conversation %s:\n%s", b.chatID, user.SelectedConversation, err)
					b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, response+"\n\n"+"sending audio response:\n"+err.Error(), nil, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})

				} else {
					b.DeleteMessage(b.chatID, user.lastSentMessageID)
				}
			}
		} else {
			b.sendMessageToUserChan(&user.lastSentMessageIDOption, b.chatID, response, nil, &echotron.MessageTextOptions{ParseMode: echotron.Markdown})
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

	b.sendMessageToAdminChan(nil, fmt.Sprintf("Nuovo utente non revisionato: %d", b.chatID),
		&echotron.MessageOptions{ReplyMarkup: echotron.InlineKeyboardMarkup{
			InlineKeyboard: [][]echotron.InlineKeyboardButton{
				{
					echotron.InlineKeyboardButton{Text: "Whitelist", CallbackData: fmt.Sprintf("/whitelist %d", b.chatID)},
					echotron.InlineKeyboardButton{Text: "Blacklist", CallbackData: fmt.Sprintf("/blacklist %d", b.chatID)},
				},
			},
		}},
		nil,
	)
	if err != nil {
		b.loggingChannel <- fmt.Sprintf("Failed to send notify admin for user %d: %v", b.chatID, err)
	}
}

func (b *Bot) transcript(fileID string) (string, error) {
	var path = ""
	b.loggingChannel <- fmt.Sprintf("Transcribing message received from: " + strconv.FormatInt(b.chatID, 10))

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

	jsonData, err := json.MarshalIndent(b.users, "", "  ")
	if err != nil {
		b.loggingChannel <- fmt.Errorf("failed to marshal users map: %w", err).Error()
		return err
	}

	if err := createDirIfNotExist(fullFilePath); err != nil {
		b.loggingChannel <- fmt.Errorf("failed to create directory: %w", err).Error()
		return err
	}

	err = os.WriteFile(fullFilePath, jsonData, 0644)
	if err != nil {
		b.loggingChannel <- fmt.Errorf("failed to write file: %w", err).Error()
		return err
	}

	return nil
}

func (b *Bot) loadUsers() error {
	b.loggingChannel <- "Loading users..."

	_, err := os.Stat(fullFilePath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	jsonData, err := os.ReadFile(fullFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	err = json.Unmarshal(jsonData, &b.users)
	if err != nil {
		return fmt.Errorf("failed to unmarshal b.users: %w", err)
	}

	b.initializeUsers()

	return nil
}

func (b *Bot) initializeUsers() {
	b.loggingChannel <- "Initializing users..."
	for _, user := range b.users {
		if b.users[user.ChatID].messageChan == nil {
			b.users[user.ChatID].messageChan = make(chan *msgCtx)
			go b.users[user.ChatID].startMessagesChannel()
		}
	}
}
