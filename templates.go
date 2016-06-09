package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type templateSet struct {
	root  string
	data  *template.Template
	mtime time.Time
}

type viewmodel interface {
	TemplateFile() string
}

var htmlInterElementWhitespace = regexp.MustCompile(">\\s+<")

func (ts *templateSet) Render(w http.ResponseWriter, code int, vm viewmodel) error {
	name := vm.TemplateFile()
	stat, err := os.Stat(filepath.Join(ts.root, name))
	if ts.data == nil || (err == nil && stat.ModTime().After(ts.mtime)) {
		ts.data, err = template.ParseGlob(filepath.Join(ts.root, "*"))
		if err != nil {
			return err
		}
		ts.mtime = time.Now()
	}
	if t := ts.data.Lookup(name); t != nil {
		buf := &bytes.Buffer{}
		if err = t.Execute(buf, vm); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/html; encoding=utf-8")
		w.WriteHeader(code)
		w.Write(htmlInterElementWhitespace.ReplaceAll(buf.Bytes(), []byte("> <")))
		return nil
	}
	return fmt.Errorf("template not found: %s", name)
}

var Render = (&templateSet{root: "templates"}).Render

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

func RenderError(w http.ResponseWriter, code int, message string) error {
	w.Header().Set("Cache-Control", "no-cache")
	return Render(w, code, ErrorTemplate{code, message})
}

func RenderInvalidMethod(w http.ResponseWriter, methods string) error {
	w.Header().Set("Allow", methods)
	return RenderError(w, http.StatusMethodNotAllowed, "")
}

type Landing struct {
	User *UserData
}

func (_ Landing) TemplateFile() string {
	return "landing.html"
}

type Room struct {
	ID       string
	Editable bool
	Online   bool
	Meta     *StreamMetadata
	User     *UserData
}

func (_ Room) TemplateFile() string {
	return "room.html"
}

func (_ Room) Live() bool {
	return true
}

type Recordings struct {
	ID    string
	Owned bool
	User  *UserData
	*StreamHistory
}

func (_ Recordings) TemplateFile() string {
	return "recordings.html"
}

type Recording struct {
	ID       string
	Editable bool // false
	Online   bool // false
	Meta     *StreamRecording
	User     *UserData
}

func (_ Recording) TemplateFile() string {
	return "room.html"
}

func (r Recording) Live() bool {
	return false
}

type UserNew int
type UserLogin int
type UserRestore int
type UserConfig struct {
	User *UserData
}

func (_ UserNew) TemplateFile() string {
	return "user-new.html"
}

func (_ UserLogin) TemplateFile() string {
	return "user-login.html"
}

func (_ UserRestore) TemplateFile() string {
	return "user-restore.html"
}

func (_ UserConfig) TemplateFile() string {
	return "user-config.html"
}
