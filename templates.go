package main

import (
	"bytes"
	"github.com/oxtoacart/bpool"
	"golang.org/x/exp/inotify"
	"html/template"
	"log"
	"net/http"
)

func loadTemplates(dir string, watch bool) *template.Template {
	t := template.Must(template.ParseGlob(dir + "/*"))
	if watch {
		watcher, err := inotify.NewWatcher()
		if err != nil {
			panic("inotify error: " + err.Error())
		}
		if err = watcher.Watch(dir); err != nil {
			panic("inotify error: " + err.Error())
		}
		go func() {
			for {
				select {
				case ev := <-watcher.Event:
					if ev.Mask&(inotify.IN_MODIFY|inotify.IN_CREATE) != 0 {
						t2, err2 := template.ParseGlob(dir + "/*")
						if err2 != nil {
							log.Println("could not reload templates: ", err2)
							continue
						}
						*t = *t2
					}
				case err := <-watcher.Error:
					log.Println("inotify error: ", err)
				}
			}
		}()
	}
	return t
}

var bufpool = bpool.NewBufferPool(64)
var templates = loadTemplates("templates", true)

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

func Render(name string, data interface{}) (*bytes.Buffer, error) {
	buf := bufpool.Get()
	err := templates.ExecuteTemplate(buf, name, data)
	return buf, err
}

func RenderHtml(w http.ResponseWriter, code int, template string, data interface{}) error {
	buf, err := Render(template, data)
	defer bufpool.Put(buf)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(code)
	buf.WriteTo(w)
	return nil
}

func RenderError(w http.ResponseWriter, code int, message string) error {
	w.Header().Set("Cache-Control", "no-cache")
	return RenderHtml(w, code, "error.html", errorViewModel{code, message})
}
