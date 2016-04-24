package templates

import "../database"

type Room struct {
	ID     string
	Owned  bool
	Online bool
	Meta   *database.StreamMetadata
	User   *database.UserShortData
}

func (_ Room) TemplateFile() string {
	return "room.html"
}
