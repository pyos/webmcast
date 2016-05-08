package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
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
		if err = RenderError(w, http.StatusInternalServerError, ""); err != nil {
			log.Println("error rendering error", err.Error())
			http.Error(w, "Error while rendering error message.", http.StatusInternalServerError)
		}
	}
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	bind := flag.String("bind", ":8000", "[network]:port to bind on")
	addr := flag.String("addr", "", "The public address of this server, which will run in pure retransmission mode.")
	ui_mode := flag.Bool("ui", false, "Run in pure ui node. Required to use ephemeral storage.")
	devnull := flag.Bool("ephemeral", false, "Use a process-local in-memory userless database. Can only be enabled in joint mode.")
	flag.Parse()

	if *devnull && (*ui_mode || *addr != "") {
		log.Print("-ephemeral cannot be used with -ui or -addr")
		log.Fatal("These modes require coordination through a persistent database.")
	}

	ctx := Context{
		SecureKey:       []byte("12345678901234567890123456789012"),
		StreamKeepAlive: 10 * time.Second,
	}
	if *devnull {
		ctx.Database = NewAnonDatabase()
	} else {
		var err error
		if ctx.Database, err = NewSQLDatabase(*addr, "sqlite3", "development.db"); err != nil {
			log.Fatal("Could not connect to database: ", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(disallowDirectoryListing{http.Dir(".")}))
	if *addr != "" || !*ui_mode {
		mux.Handle("/stream/", http.StripPrefix("/stream", UnsafeHandler{NewRetransmissionContext(&ctx)}))
	}
	if *addr == "" {
		mux.Handle("/", UnsafeHandler{NewUIHandler(&ctx)})
	}
	log.Fatal(http.ListenAndServe(*bind, mux))
}
