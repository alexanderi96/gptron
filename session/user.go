package session

import (
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
