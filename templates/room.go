package templates

import (
	"../chat"
	"../database"
)

type Room struct {
	ID     string
	Owned  bool
	Online bool
	Meta   *database.StreamMetadata
	User   *database.UserShortData
	Chat   *chat.Context
}

func (_ Room) TemplateFile() string {
	return "room.html"
}
