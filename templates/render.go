package templates

import (
	"fmt"
	"github.com/oxtoacart/bpool"
	"html/template"
	"log"
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

type disallowDirectoryListing struct {
	http.FileSystem
}

func (fs disallowDirectoryListing) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if stat, _ := f.Stat(); stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, nil
}

func StaticHandler(root string) http.Handler {
	return http.FileServer(disallowDirectoryListing{http.Dir(root)})
}

type UnsafeHandlerIface interface {
	ServeHTTPUnsafe(w http.ResponseWriter, r *http.Request) error
}

type UnsafeHandler struct {
	UnsafeHandlerIface
}

func (ctx UnsafeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		r.Method = "GET"
	}
	if err := ctx.ServeHTTPUnsafe(w, r); err != nil {
		log.Println("error rendering template", r.URL.Path, err.Error())
		if err = Error(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}
