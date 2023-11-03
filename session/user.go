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

	MenuStateMain     = "main"
	MenuStateList     = "list"
	MenuStateSelected = "selected"
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
	ID            uuid.UUID
	Title         string
	Content       *openai.ChatCompletionRequest
	UserRole      string
	AssistantRole string
	CreationTime  time.Time
	LastUpdate    time.Time
}

func (c *Conversation) AppendMessage(message, role string) {
	c.Content.Messages = append(c.Content.Messages, openai.ChatCompletionMessage{
		Role:    role,
		Content: message,
	})
	c.LastUpdate = time.Now()
}

func (u *User) CreateNewConversation(engineID, userID string) {
	convUuid := uuid.New()
	u.Conversations[convUuid] = &Conversation{
		ID: convUuid,
		Content: &openai.ChatCompletionRequest{
			Model:    engineID,
			Messages: make([]openai.ChatCompletionMessage, 0),
			User:     userID,
		},
		UserRole:      openai.ChatMessageRoleUser,
		AssistantRole: openai.ChatMessageRoleAssistant,
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
	for _, message := range c.Content.Messages {
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

		if longestConv == nil || len(longestConv.Content.Messages) < len(conv.Content.Messages) {
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
