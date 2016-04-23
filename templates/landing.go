package templates

import "../database"

type Landing struct {
	User *database.UserShortData
}

func (_ Landing) TemplateFile() string {
	return "landing.html"
}
