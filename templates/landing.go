package templates

import "../common"

type Landing struct {
	User *common.UserShortData
}

func (_ Landing) TemplateFile() string {
	return "landing.html"
}
