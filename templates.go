package main

import (
	"fmt"
	"html/template"
	"net/http"
)

var templates = template.Must(template.ParseFiles(
	"templates/room.html",
	"templates/error.html",
))

type roomViewModel struct {
	ID     string
	Stream *BroadcastContext
}

type errorViewModel struct {
	Code    int
	Message string
}

func (e errorViewModel) DisplayMessage() string {
	if e.Message != "" {
		return e.Message
	}

	switch e.Code {
	case 403:
		return "FOREBODEN."
	case 404:
		return "There is nothing here."
	case 418:
		return "I'm a little teapot."
	case 500:
		return "✋☠❄☜☼☠✌☹ 💧☜☼✞☜☼ ☜☼☼⚐☼"
	default:
		return "ERROR"
	}
}

func (e errorViewModel) DisplayComment() string {
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

func Render(w http.ResponseWriter, name string, data interface{}) {
	err := templates.ExecuteTemplate(w, name, data)
	if err != nil {
		if name == "error.html" {
			fmt.Fprintf(w, "Error while rendering error: %s", err.Error())
		} else {
			Render(w, "error.html", errorViewModel{500, err.Error()})
		}
	}
}

func RenderHtml(w http.ResponseWriter, code int, template string, data interface{}) {
	header := w.Header()
	header["Content-Type"] = []string{"text/html"}
	w.WriteHeader(code)
	Render(w, template, data)
}

func RenderError(w http.ResponseWriter, code int, message string) {
	header := w.Header()
	header["Content-Type"] = []string{"text/html"}
	header["Cache-Control"] = []string{"no-cache"}
	w.WriteHeader(code)
	Render(w, "error.html", errorViewModel{code, message})
}
