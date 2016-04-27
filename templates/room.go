package templates

import "../common"

type Room struct {
	ID     string
	Owned  bool
	Online bool
	Meta   *common.StreamMetadata
	User   *common.UserData
}

func (_ Room) TemplateFile() string {
	return "room.html"
}
