package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"./broadcast"
	"./common"
	"./templates"
	"./ui"
)

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
		if err = templates.Error(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	bind := flag.String("bind", ":8000", "[network]:port to bind on")
	addr := flag.String("addr", "", "Public address of this node (enables root mode)")
	flag.Parse()

	d, err := common.NewSQLDatabase(*addr, "sqlite3", "development.db")
	if err != nil {
		log.Fatal("could not connect to database:", d)
	}

	ctx := common.Context{
		Database:        d,
		SecureKey:       []byte("12345678901234567890123456789012"),
		StreamKeepAlive: 10 * time.Second,
	}

	var handler UnsafeHandlerIface
	if *addr != "" {
		handler = broadcast.NewHTTPContext(&ctx)
	} else {
		handler = ui.NewHTTPContext(&ctx)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(disallowDirectoryListing{http.Dir(".")}))
	mux.Handle("/", UnsafeHandler{handler})
	log.Fatal(http.ListenAndServe(*bind, mux))
}
