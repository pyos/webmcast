package templates

import (
	"../broadcast"
	"../chat"
	"../database"
)

type Room struct {
	ID     string
	Owned  bool
	Stream *broadcast.Broadcast
	Meta   *database.StreamMetadata
	User   *database.UserShortData
	Chat   *chat.Context
}

func (_ Room) TemplateFile() string {
	return "room.html"
}
