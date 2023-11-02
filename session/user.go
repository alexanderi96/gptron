package session

import (
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
	Status               UserStatus
	IsAdmin              bool
	Conversations        map[uuid.UUID]*Conversation
	SelectedConversation uuid.UUID `json:"-"`
	MenuState            string    `json:"-"`
}

type Conversation struct {
	ID      uuid.UUID
	Content *openai.ChatCompletionRequest
}

func createMessage(text string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: text,
	}
}

func (u *User) AppendMessageAndGetConversation(text string) *[]openai.ChatCompletionMessage {
	u.Conversations[u.SelectedConversation].Content.Messages = append([]openai.ChatCompletionMessage{*createMessage(text)}, u.Conversations[u.SelectedConversation].Content.Messages...)

	return &u.Conversations[u.SelectedConversation].Content.Messages
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
	}
	u.SelectedConversation = convUuid
}
