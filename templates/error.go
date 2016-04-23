package templates

import "net/http"

type ErrorTemplate struct {
	Code    int
	Message string
}

func (_ ErrorTemplate) TemplateFile() string {
	return "error.html"
}

func (e ErrorTemplate) DisplayMessage() string {
	if e.Message != "" {
		return e.Message
	}

	switch e.Code {
	case 403:
		return "FOREBODEN."
	case 404:
		return "There is nothing here."
	case 405:
		return "Invalid HTTP method."
	case 418:
		return "I'm a little teapot."
	case 500:
		return "✋☠❄☜☼☠✌☹ 💧☜☼✞☜☼ ☜☼☼⚐☼"
	default:
		return "ERROR"
	}
}

func (e ErrorTemplate) DisplayComment() string {
	switch e.Code {
	case 403:
		return "you're just a dirty hacker, aren't you?"
	case 404:
		return "(The dog absorbs the page.)"
	case 418:
		return "Would you like a cup of tea?"
	case 500:
		return "Try submitting a bug report."
	default:
		return "Try something else."
	}
}

func Error(w http.ResponseWriter, code int, message string) error {
	w.Header().Set("Cache-Control", "no-cache")
	return Page(w, code, ErrorTemplate{code, message})
}

func InvalidMethod(w http.ResponseWriter, methods string) error {
	w.Header().Set("Allow", methods)
	return Error(w, http.StatusMethodNotAllowed, "")
}
