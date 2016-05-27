package main

import (
	"flag"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

type disallowDirectoryListing http.Dir

func (fs disallowDirectoryListing) Open(name string) (http.File, error) {
	f, err := http.Dir(fs).Open(name)
	if err != nil {
		return nil, err
	}
	if stat, _ := f.Stat(); stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, nil
}

type UnsafeHandlerIface interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request) error
}

type UnsafeHandler struct {
	UnsafeHandlerIface
}

func (ctx UnsafeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		r.Method = "GET"
	}
	if err := ctx.UnsafeHandlerIface.ServeHTTP(w, r); err != nil {
		log.Println("error rendering template", r.URL.Path, err.Error())
		if err = RenderError(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	bind := flag.String("bind", ":8000", "The network ([ip]:port) to bind on.")
	addr := flag.String("addr", "", "The public address (host[:port]) of this node.")
	ephemeral := flag.Bool("ephemeral", false, "Use a process-local in-memory userless database. Can only be enabled in joint mode.")
	flag.Parse()

	if *ephemeral && *addr != "" {
		log.Fatal("-ephemeral cannot be used with -addr. Running as a part of a cluster requires coordination through a database.")
	}

	ctx := Context{
		Database:        NewAnonDatabase(),
		SecureKey:       []byte("12345678901234567890123456789012"),
		StreamKeepAlive: 10 * time.Second,
	}
	if !*ephemeral {
		var err error
		if ctx.Database, err = NewSQLDatabase(*addr, "sqlite3", "development.db"); err != nil {
			log.Fatal("Could not connect to database: ", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(disallowDirectoryListing(".")))
	mux.Handle("/stream/", UnsafeHandler{NewRetransmissionHandler(&ctx)})
	mux.Handle("/", UnsafeHandler{NewUIHandler(&ctx)})
	log.Fatal(http.ListenAndServe(*bind, mux))
}
