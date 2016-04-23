package templates

import "../database"

type UserLogin int
type UserSignup int
type UserRestore int
type UserConfig struct {
	User *database.UserMetadata
}

func (_ UserLogin) TemplateFile() string {
	return "user-login.html"
}

func (_ UserSignup) TemplateFile() string {
	return "user-signup.html"
}

func (_ UserRestore) TemplateFile() string {
	return "user-restore.html"
}

func (_ UserConfig) TemplateFile() string {
	return "user-config.html"
}
