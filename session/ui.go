package session

import (
	"github.com/NicoNex/echotron/v3"
	"github.com/alexanderi96/gptron/gpt"
)

func getMainMenu(isAdmin bool) *echotron.ReplyKeyboardMarkup {
	kbd := &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/list", RequestContact: false, RequestLocation: false},
				{Text: "/new", RequestContact: false, RequestLocation: false},
			},
			{
				{Text: "/settings", RequestContact: false, RequestLocation: false},
				{Text: "/stats", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}

	if isAdmin {
		kbd.Keyboard = append(kbd.Keyboard,
			[]echotron.KeyboardButton{
				{Text: "/users_list", RequestContact: false, RequestLocation: false},
				{Text: "/global_stats", RequestContact: false, RequestLocation: false},
			})
	}
	return kbd
}

func (u *User) getListOfChats() *echotron.ReplyKeyboardMarkup {
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

	for _, conv := range u.Conversations {
		command := "/select "
		if conv.Title == "" {
			command += conv.ID.String()
		} else {
			command += conv.Title + " " + conv.ID.String()
		}
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: command}})
	}

	return menu
}
func getConversationUI() *echotron.ReplyKeyboardMarkup {
	menu := &echotron.ReplyKeyboardMarkup{
		Keyboard: [][]echotron.KeyboardButton{
			{
				{Text: "/back", RequestContact: false, RequestLocation: false},
				{Text: "/stats", RequestContact: false, RequestLocation: false},
			},
			{
				{Text: "/summarize", RequestContact: false, RequestLocation: false},
				{Text: "/delete", RequestContact: false, RequestLocation: false},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
		Selective:       false,
	}

	return menu
}

func getPersonalityList() *echotron.ReplyKeyboardMarkup {
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

	for key := range gpt.Personalities {
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: "/ask " + key}})
	}

	return menu
}

func getModelList() *echotron.ReplyKeyboardMarkup {
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

	for _, model := range gpt.Models {
		menu.Keyboard = append(menu.Keyboard, []echotron.KeyboardButton{{Text: "/model " + model.Name}})
	}

	return menu
}
