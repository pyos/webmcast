package templates

import (
	"fmt"
	"github.com/oxtoacart/bpool"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	root      = "templates"
	buffers   = bpool.NewBufferPool(64)
	parsed    *template.Template
	lastParse time.Time
)

type item interface {
	TemplateFile() string
}

func Page(w http.ResponseWriter, code int, data item) error {
	name := data.TemplateFile()
	stat, err := os.Stat(filepath.Join(root, name))
	if parsed == nil || (err == nil && stat.ModTime().After(lastParse)) {
		parsed, err = template.ParseGlob(filepath.Join(root, "*"))
		if err != nil {
			return err
		}
		lastParse = time.Now()
	}
	if t := parsed.Lookup(name); t != nil {
		buf := buffers.Get()
		defer buffers.Put(buf)
		if err = t.Execute(buf, data); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/html; encoding=utf-8")
		w.WriteHeader(code)
		buf.WriteTo(w)
		return nil
	}
	return fmt.Errorf("template not found: %s", name)
}
