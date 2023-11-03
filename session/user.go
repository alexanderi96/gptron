package session

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

type UserStatus int

const (
	Unreviewed UserStatus = iota
	Whitelisted
	Blacklisted

	MenuStateMain              = "main"
	MenuStateList              = "list"
	MenuStateSelected          = "selected"
	MenuStateSelectPersonality = "select_personality"
)

type User struct {
	ChatID               int64
	Status               UserStatus
	IsAdmin              bool
	Conversations        map[uuid.UUID]*Conversation
	SelectedConversation uuid.UUID `json:"-"`
	MenuState            string    `json:"-"`
	CreationTime         time.Time
	LastUpdate           time.Time
}

type Conversation struct {
	ID             uuid.UUID
	Title          string
	Content        []*Message
	UserRole       string
	AssistantRole  string
	Model          string
	GptPersonality string
	CreationTime   time.Time
	LastUpdate     time.Time
}

type Message struct {
	Role         string
	Content      string
	CreationTime time.Time
}

func (c *Conversation) AppendMessage(text, role string) {
	c.Content = append(c.Content, &Message{
		Role:         role,
		Content:      text,
		CreationTime: time.Now(),
	})
	c.LastUpdate = time.Now()
}

func (u *User) CreateNewConversation(engineID, userID string) {
	convUuid := uuid.New()
	u.Conversations[convUuid] = &Conversation{
		ID:            convUuid,
		Content:       make([]*Message, 0),
		UserRole:      openai.ChatMessageRoleUser,
		AssistantRole: openai.ChatMessageRoleAssistant,
		Model:         engineID,
		CreationTime:  time.Now(),
		LastUpdate:    time.Now(),
	}
	u.SelectedConversation = convUuid
}

func NewUser(admin bool, userID int64) *User {
	return &User{
		ChatID:               userID,
		Status:               Unreviewed,
		IsAdmin:              admin,
		Conversations:        make(map[uuid.UUID]*Conversation),
		SelectedConversation: uuid.Nil,
		CreationTime:         time.Now(),
		LastUpdate:           time.Now(),
	}
}

func (c *Conversation) TokenCount() (int, int) {
	inputTokens := 0
	outputTokens := 0
	for _, message := range c.Content {
		tokenCount := len(message.Content) // Semplice conteggio dei token basato sulla lunghezza del messaggio
		if message.Role == c.UserRole {
			inputTokens += tokenCount
		} else if message.Role == c.AssistantRole {
			outputTokens += tokenCount
		}
	}
	return inputTokens, outputTokens
}

func (u *User) GetConversationStats(convID uuid.UUID) string {
	conv, exists := u.Conversations[convID]
	if !exists {
		return "Conversazione non trovata."
	}
	inputTokens, outputTokens := conv.TokenCount()
	return fmt.Sprintf("Conversazione: %s\nToken in ingresso: %d\nToken in uscita: %d\n", conv.Title, inputTokens, outputTokens)
}

func (u *User) GetGlobalStats() string {
	totalInputTokens := 0
	totalOutputTokens := 0
	var longestConv *Conversation
	var oldestConv *Conversation

	for _, conv := range u.Conversations {
		inputTokens, outputTokens := conv.TokenCount()
		totalInputTokens += inputTokens
		totalOutputTokens += outputTokens

		if longestConv == nil || len(longestConv.Content) < len(conv.Content) {
			longestConv = conv
		}

		if oldestConv == nil || oldestConv.CreationTime.Before(conv.CreationTime) {
			oldestConv = conv
		}
	}

	stats := fmt.Sprintf("Statistiche Globali:\n")
	stats += fmt.Sprintf("Token totali in ingresso: %d\n", totalInputTokens)
	stats += fmt.Sprintf("Token totali in uscita: %d\n", totalOutputTokens)
	if longestConv != nil {
		stats += fmt.Sprintf("Conversazione più lunga: %s\n", longestConv.Title)
	}
	if oldestConv != nil {
		stats += fmt.Sprintf("Conversazione più vecchia: %s\n", oldestConv.Title)
	}
	return stats
}

func (c *Conversation) GetChatCompletionRequest() *openai.ChatCompletionRequest {
	ctx := &openai.ChatCompletionRequest{
		Model:    c.Model,
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
