package session

import (
	"fmt"
	"log"
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
	ChatID               int64
	Status               UserStatus
	Conversations        map[uuid.UUID]*Conversation
	SelectedConversation uuid.UUID `json:"-"`
	MenuState            MenuState `json:"-"`
	CreationTime         time.Time
	LastUpdate           time.Time
	StrikeCount          int
	UsageMap             map[string]gpt.Model
	messageChan          chan *msgCtx
	replyWithVoice       bool
}

type msgCtx struct {
	bot         *Bot
	msg         string
	replyMarkup *echotron.MessageOptions
}

func (u *User) startMessagesChannel() {
	log.Println("Starting messaging channel")

	baseMessage := "MESSAGE CHANNEL:\n\n"
	defer close(u.messageChan) // Chiude il canale quando la funzione termina

	for ctx := range u.messageChan {
		ctx.bot.loggingChannel <- fmt.Sprintf("sending message to user %v", u.ChatID)
		ctx.bot.SendMessage(baseMessage+ctx.msg, u.ChatID, ctx.replyMarkup)
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
		return "Select a model from the list", getModelList()
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
	u.Conversations[u.SelectedConversation].Model = convModel

	model, exists := u.UsageMap[convModel.Name]
	if !exists {
		u.UsageMap[convModel.Name] = convModel
	} else {
		model.Usage.PromptTokens += promptTokens
		model.Usage.CompletionTokens += completionTokens
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
		gpt.SummarizatorPrompt(c.Model.Name, ctx.Messages),
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
	resp, err := gpt.SendMessagesToChatGPT(gpt.GetTitleContext(c.Model.Name, c.GetChatCompletionRequest().Messages))

	if err != nil {
		log.Print("Title generation error: ", err)
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

func (u *User) sendMessagesToChatGPT(text string) (string, error) {
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

	log.Println("setting title")
	if conv.Title == "" {
		usage, err := conv.setTitle()
		if err != nil {
			log.Println("unable to set title:", err)
		}
		log.Println("title set")
		u.updateTokenUsage(usage.PromptTokens, usage.CompletionTokens)
	}

	return resp.Message.Content, nil
}

func NewUser(admin bool, userID int64) *User {
	status := Unreviewed
	if admin {
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
		"Costo totale: $%f", conv.Title, conv.GptPersonality, conv.Model.Name, conv.Model.Usage.PromptTokens, costs.Prompt, conv.Model.Usage.CompletionTokens, costs.Completion, costs.Prompt+costs.Completion)

}

func (u *User) GetGlobalStats() string {
	var longestConv *Conversation
	var oldestConv *Conversation

	for _, conv := range u.Conversations {

		if longestConv == nil || len(longestConv.Content) < len(conv.Content) {
			longestConv = conv
		}

		if oldestConv == nil || oldestConv.CreationTime.Before(conv.CreationTime) {
			oldestConv = conv
		}
	}

	stats := "Statistiche Globali:\n"

	for _, model := range u.UsageMap {
		cost := model.GetCosts()
		stats += fmt.Sprintf("Engine: %s\n", model.Name)
		stats += fmt.Sprintf("Token in ingresso: %d ($%f)\n", model.Usage.PromptTokens, cost.Prompt)
		stats += fmt.Sprintf("Token in uscita: %d ($%f)\n", model.Usage.CompletionTokens, cost.Completion)
		stats += "\n"
	}

	tokens := u.GetTotalTokens()
	costs := u.GetTotalCost()

	stats += fmt.Sprintf("Totale token in ingresso: %d\n", tokens.PromptTokens)
	stats += fmt.Sprintf("Totale token in uscita: %d\n", tokens.CompletionTokens)
	stats += "\n"
	stats += fmt.Sprintf("Costo token ingresso: $%f\n", costs.Prompt)
	stats += fmt.Sprintf("Costo token uscita: $%f\n", costs.Completion)
	stats += "\n"
	stats += fmt.Sprintf("*Costo totale*: $%f\n", costs.Prompt+costs.Completion)
	stats += "\n"

	if longestConv != nil {
		stats += fmt.Sprintf("Conversazione più lunga:%s\n\n", longestConv.Title)
	}
	if oldestConv != nil {
		stats += fmt.Sprintf("Conversazione più vecchia:\n%s\n\n", oldestConv.Title)
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
