package main

import (
	"github.com/oxtoacart/bpool"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type templateItem struct {
	parsed *template.Template
	mtime  time.Time
}

type templateSet struct {
	root   string
	loaded map[string]templateItem
}

func (ts *templateSet) Get(name string) (*template.Template, error) {
	path := filepath.Join(ts.root, name)
	stat, err := os.Stat(path)
	if t, ok := ts.loaded[name]; ok && !(err == nil && stat.ModTime().After(t.mtime)) {
		return t.parsed, nil
	}
	t, err := template.ParseFiles(path)
	if err != nil {
		return nil, err
	}
	ts.loaded[name] = templateItem{t, stat.ModTime()}
	return t, nil
}

type roomViewModel struct {
	ID     string
	Stream *BroadcastContext
	Meta   *StreamMetadata
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

var bufpool = bpool.NewBufferPool(64)
var templates = templateSet{"templates", make(map[string]templateItem)}

func Render(w http.ResponseWriter, code int, template string, data interface{}) error {
	tpl, err := templates.Get(template)
	if err != nil {
		return err
	}
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	if err = tpl.Execute(buf, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; encoding=utf-8")
	w.WriteHeader(code)
	buf.WriteTo(w)
	return nil
}

func RenderError(w http.ResponseWriter, code int, message string) error {
	w.Header().Set("Cache-Control", "no-cache")
	return Render(w, code, "error.html", errorViewModel{code, message})
}
