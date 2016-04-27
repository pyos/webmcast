package templates

import "../common"

type Landing struct {
	User *common.UserData
}

func (_ Landing) TemplateFile() string {
	return "landing.html"
}
