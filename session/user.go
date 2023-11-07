package session

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/NicoNex/echotron/v3"
	"github.com/alexanderi96/gptron/gpt"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

type UserStatus string
type MenuState string

const (
	Unreviewed  UserStatus = "🕒"
	Whitelisted UserStatus = "✅"
	Blacklisted UserStatus = "❌"
	Admin       UserStatus = "👑"

	MenuStateMain              MenuState = "main"
	MenuStateList              MenuState = "list"
	MenuStateSelected          MenuState = "selected"
	MenuStateSelectPersonality MenuState = "select_personality"
	MenuStateSelectModel       MenuState = "select_model"

	maxUsage = 1 //imposto il consumo massimo per utente ad 1$
)

type Message struct {
	Role         string
	Content      string
	CreationTime time.Time
}

// do not delete the conversation, in order to keep track of costs ecc
// just delete messages, title and roles and add a variable
// in order to tell if the chat is active or not. and also track the deletion time
type Conversation struct {
	ID             uuid.UUID
	Title          string
	Content        []*Message
	Model          gpt.Model
	GptPersonality string
	CreationTime   time.Time
	LastUpdate     time.Time
	DeletionTime   time.Time
	Deleted        bool
}

type User struct {
	ChatID                  int64
	Status                  UserStatus
	Conversations           map[uuid.UUID]*Conversation
	SelectedConversation    uuid.UUID
	MenuState               MenuState
	CreationTime            time.Time
	LastUpdate              time.Time
	StrikeCount             int
	UsageMap                map[string]gpt.Model
	messageChan             chan *msgCtx
	replyWithVoice          bool
	lastSentMessageID       int
	lastSentMessageIDOption echotron.MessageIDOptions
}

type msgCtx struct {
	initialMessageID   *echotron.MessageIDOptions
	bot                *Bot
	chatID             int64
	msg                string
	messageOptions     *echotron.MessageOptions
	messageTextOptions *echotron.MessageTextOptions
}

func (c *Conversation) GenerateReport() string {
	report := fmt.Sprintf(
		"## Conversation Report\n"+
			"- **Conversation ID:** %s\n"+
			"- **Title:** %s\n"+
			"- **Personality:** %s\n"+
			"- **Creation Time:** %s\n"+
			"- **Last Update:** %s\n\n",
		c.ID,
		c.Title,
		c.GptPersonality,
		c.CreationTime.Format("2006-01-02 15:04:05"),
		c.LastUpdate.Format("2006-01-02 15:04:05"),
	)

	cost := c.Model.GetCosts()

	report += fmt.Sprintf(
		"- **Model:** %s\n"+
			"- **Total tokens:** %d ($%f)\n\n",
		c.Model.Name,
		c.Model.Usage.TotalTokens,
		cost.Prompt+cost.Completion,
	)

	report += "### Messages\n\n"
	for i, message := range c.Content {
		report += fmt.Sprintf(
			"**%s** - **%s**:\n\n%s\n\n",
			message.CreationTime.Format("2006-01-02 15:04:05"),
			message.Role,
			message.Content,
		)
		if i != len(c.Content)-1 {
			report += "---\n\n"
		}
	}

	if c.Deleted {
		report += fmt.Sprintf(
			"### Deletion Details\n\n"+
				"The conversation was deleted at: %s\n",
			c.DeletionTime.Format("2006-01-02 15:04:05"),
		)
	}

	return report
}

func (u *User) startMessagesChannel() {
	defer close(u.messageChan) // Chiude il canale quando la funzione termina

	baseMessage := ""

	for ctx := range u.messageChan {
		b := ctx.bot

		if ctx.initialMessageID != nil {
			b.EditMessageText(baseMessage+ctx.msg, u.lastSentMessageIDOption, ctx.messageTextOptions)
		} else {

			initialMessage, err := b.SendMessage(baseMessage+ctx.msg, ctx.chatID, ctx.messageOptions)

			if err != nil {
				b.loggingChannel <- fmt.Sprintf("Error sending message to %d: %s", ctx.chatID, err)
				return
			}

			u.lastSentMessageID = initialMessage.Result.ID
			u.lastSentMessageIDOption = echotron.NewMessageID(ctx.chatID, initialMessage.Result.ID)

		}

	}

}

func (u *User) HasReachedUsageLimit() bool {
	costs := u.GetTotalCost()
	return costs.Completion+costs.Prompt >= maxUsage
}

func (u *User) IsAdmin() bool {
	return u.Status == Admin
}

// Funzione per cambiare il menu dell'utente
func (u *User) GetMenu() (string, echotron.ReplyMarkup) {
	switch u.MenuState {
	case MenuStateList:
		replyMarkup := u.getListOfChats()
		return "Select a conversation from the list", replyMarkup
	case MenuStateSelected:
		return "Conversation Menu", getConversationUI()
	case MenuStateSelectPersonality:
		return "Select a personality from the list", getPersonalityList()
	case MenuStateSelectModel:
		return "Select a model from the list", getModelList(u.IsAdmin())
	default:
		return "Main Menu", getMainMenu(u.IsAdmin())
	}
}

func (c *Conversation) AppendMessage(text, role string) {
	c.Content = append(c.Content, &Message{
		Role:         role,
		Content:      text,
		CreationTime: time.Now(),
	})
	c.LastUpdate = time.Now()
}

func (u *User) updateTokenUsage(promptTokens, completionTokens int) {
	convModel := u.Conversations[u.SelectedConversation].Model
	convModel.Usage.PromptTokens += promptTokens
	convModel.Usage.CompletionTokens += completionTokens
	convModel.Usage.TotalTokens = convModel.Usage.PromptTokens + convModel.Usage.CompletionTokens
	u.Conversations[u.SelectedConversation].Model = convModel

	model, exists := u.UsageMap[convModel.Name]
	if !exists {
		u.UsageMap[convModel.Name] = convModel
	} else {
		model.Usage.PromptTokens += promptTokens
		model.Usage.CompletionTokens += completionTokens
		model.Usage.TotalTokens = model.Usage.PromptTokens + model.Usage.CompletionTokens
		u.UsageMap[convModel.Name] = model
	}
}

func (c *Conversation) Summarize(n int) (string, error) {

	if len(c.Content) < n {
		log.Print("Not enough messages to summarize, using all messages")
		n = len(c.Content)
	}

	convBk := *c
	convBk.Content = convBk.Content[len(c.Content)-n:]

	ctx := convBk.GetChatCompletionRequest()

	// Invia la richiesta
	resp, err := gpt.SendMessagesToChatGPT(
		gpt.SummarizatorPrompt(&c.Model, ctx.Messages),
	)
	if err != nil {
		log.Print("Summarization error: ", err)
		return "", err
	}

	c.Model.Usage.PromptTokens += resp.Usage.PromptTokens
	c.Model.Usage.CompletionTokens += resp.Usage.CompletionTokens

	return resp.Message.Content, nil

}

func (c *Conversation) setTitle() (openai.Usage, error) {
	resp, err := gpt.SendMessagesToChatGPT(gpt.GetTitleContext(&c.Model, c.GetChatCompletionRequest().Messages))
	if err != nil {
		c.Title = "New Chat with " + c.GptPersonality
		return resp.Usage, err
	} else {
		c.Title = resp.Message.Content
	}
	return resp.Usage, nil
}

func (u *User) NewConversation() uuid.UUID {
	convUuid := uuid.New()
	u.Conversations[convUuid] = &Conversation{
		ID:           convUuid,
		Content:      make([]*Message, 0),
		CreationTime: time.Now(),
		LastUpdate:   time.Now(),
		Model:        gpt.Model{},
	}
	return convUuid
}

func (u *User) sendMessagesToChatGPT(text string, b *Bot) (string, error) {
	if u.SelectedConversation == uuid.Nil {
		return "", fmt.Errorf("no conversation selected")
	}
	conv := u.Conversations[u.SelectedConversation]

	conv.AppendMessage(text, openai.ChatMessageRoleUser)

	resp, err := gpt.SendMessagesToChatGPT(conv.GetChatCompletionRequest())
	if err != nil {
		return "", err
	}

	conv.AppendMessage(resp.Message.Content, openai.ChatMessageRoleAssistant)
	u.updateTokenUsage(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	u.messageChan <- &msgCtx{
		initialMessageID:   &u.lastSentMessageIDOption,
		bot:                b,
		chatID:             u.ChatID,
		msg:                "Generating title",
		messageOptions:     nil,
		messageTextOptions: nil,
	}
	b.loggingChannel <- "Generating title for user " + strconv.FormatInt(u.ChatID, 10) + "'s conversation: " + conv.ID.String()
	if conv.Title == "" {
		usage, err := conv.setTitle()
		if err != nil {
			b.loggingChannel <- "Unable to generate title for user " + strconv.FormatInt(u.ChatID, 10) + "'s conversation: " + conv.ID.String() + "\n" + err.Error()
		}
		b.loggingChannel <- "Title generate for user " + strconv.FormatInt(u.ChatID, 10) + "'s conversation: " + conv.ID.String()

		u.updateTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return resp.Message.Content, nil
}

func newUser(userID int64) *User {
	status := Unreviewed
	if adminID == userID {
		status = Admin
	}
	user := &User{
		ChatID:               userID,
		Status:               status,
		Conversations:        make(map[uuid.UUID]*Conversation),
		SelectedConversation: uuid.Nil,
		CreationTime:         time.Now(),
		LastUpdate:           time.Now(),
		StrikeCount:          0,
		UsageMap:             make(map[string]gpt.Model),
		messageChan:          make(chan *msgCtx),
		replyWithVoice:       false,
	}

	go user.startMessagesChannel()
	return user
}

func (u *User) GetConversationStats(convID uuid.UUID) string {
	conv, exists := u.Conversations[convID]
	if !exists {
		return "Conversazione non trovata."
	}

	costs := conv.Model.GetCosts()

	return fmt.Sprintf("Conversazione: %s\n"+
		"Personalità: %s\n\n"+
		"Modello: %s\n"+
		"Token in ingresso: %d ($%f)\n"+
		"Token in uscita: %d($%f)\n\n"+
		"Token totali: %d ($%f)", conv.Title, conv.GptPersonality, conv.Model.Name, conv.Model.Usage.PromptTokens, costs.Prompt, conv.Model.Usage.CompletionTokens, costs.Completion, conv.Model.Usage.PromptTokens+conv.Model.Usage.CompletionTokens, costs.Prompt+costs.Completion)

}

func (u *User) GetStatsForAllChats() string {
	var longestConv *Conversation
	var shortestConv *Conversation
	var newestConv *Conversation
	var oldestConv *Conversation

	for _, conv := range u.Conversations {
		if longestConv == nil || longestConv.Model.Usage.TotalTokens < conv.Model.Usage.TotalTokens {
			longestConv = conv
		}

		if shortestConv == nil || shortestConv.Model.Usage.TotalTokens > conv.Model.Usage.TotalTokens {
			shortestConv = conv
		}

		if newestConv == nil || newestConv.CreationTime.After(conv.CreationTime) {
			newestConv = conv
		}

		if oldestConv == nil || oldestConv.CreationTime.Before(conv.CreationTime) {
			oldestConv = conv
		}
	}

	stats := "Statistiche Globali:\n"

	for _, model := range u.UsageMap {
		cost := model.GetCosts()
		stats += "\n"
		stats += fmt.Sprintf("Engine: %s\n", model.Name)
		stats += fmt.Sprintf("Token in ingresso: %d ($%f)\n", model.Usage.PromptTokens, cost.Prompt)
		stats += fmt.Sprintf("Token in uscita: %d ($%f)\n", model.Usage.CompletionTokens, cost.Completion)
		stats += fmt.Sprintf("Token totali: %d ($%f)\n", model.Usage.TotalTokens, cost.Prompt+cost.Completion)
	}

	stats += "\n-----------------------------------------------------------\n\n"

	tokens := u.GetTotalTokens()
	costs := u.GetTotalCost()

	stats += fmt.Sprintf("Totale token in ingresso: %d ($%f)\n", tokens.PromptTokens, costs.Prompt)
	stats += fmt.Sprintf("Totale token in uscita: %d ($%f)\n", tokens.CompletionTokens, costs.Completion)
	stats += "\n"
	stats += fmt.Sprintf("Token totali: %d ($%f)\n", tokens.TotalTokens, costs.Prompt+costs.Completion)
	stats += "\n"

	if longestConv != nil {
		longConvCost := longestConv.Model.GetCosts()
		stats += fmt.Sprintf("Conversazione più lunga: %s\n%d tokens ($%f)\n\n", longestConv.Title, longestConv.Model.Usage.TotalTokens, longConvCost.Prompt+longConvCost.Completion)
	}
	if shortestConv != nil {
		shortConvCost := shortestConv.Model.GetCosts()
		stats += fmt.Sprintf("Conversazione più corta: %s\n%d tokens ($%f)\n\n", shortestConv.Title, shortestConv.Model.Usage.TotalTokens, shortConvCost.Prompt+shortConvCost.Completion)
	}
	if newestConv != nil {
		newestConvCost := newestConv.Model.GetCosts()
		stats += fmt.Sprintf("Conversazione più recente: %s\n%d tokens ($%f)\n\n", newestConv.Title, newestConv.Model.Usage.TotalTokens, newestConvCost.Prompt+newestConvCost.Completion)
	}
	if oldestConv != nil {
		oldestConvCost := oldestConv.Model.GetCosts()
		stats += fmt.Sprintf("Conversazione più vecchia: %s\n%d tokens ($%f)\n\n", oldestConv.Title, oldestConv.Model.Usage.TotalTokens, oldestConvCost.Prompt+oldestConvCost.Completion)
	}
	return stats
}

func (c *Conversation) GetChatCompletionRequest() *openai.ChatCompletionRequest {
	ctx := &openai.ChatCompletionRequest{
		Model:    c.Model.Name,
		Messages: []openai.ChatCompletionMessage{},
	}

	for _, message := range c.Content {
		ctx.Messages = append(ctx.Messages, openai.ChatCompletionMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	return ctx

}

func (c *Conversation) Delete() {
	c.Deleted = true
	c.Content = []*Message{}
	c.DeletionTime = time.Now()
}

func (u *User) GetTotalTokens() (usage openai.Usage) {
	for _, conv := range u.Conversations {
		usage.PromptTokens += conv.Model.Usage.PromptTokens
		usage.CompletionTokens += conv.Model.Usage.CompletionTokens
	}
	return

}

func (u *User) GetTotalCost() (cost gpt.Pricing) {
	for _, conv := range u.Conversations {
		cost.Prompt += conv.Model.GetCosts().Prompt
		cost.Completion += conv.Model.GetCosts().Completion
	}
	return
}

func (u *User) getConversationsAsList() []*Conversation {
	list := make([]*Conversation, 0)
	for _, conv := range u.Conversations {
		list = append(list, conv)
	}
	return list
}
