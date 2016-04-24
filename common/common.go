package common

import (
	"flag"
	"time"

	"github.com/gorilla/securecookie"
	_ "github.com/mattn/go-sqlite3"
)

var (
	DefaultInterface = ":8000"
	// the key used to sign client-side secure session cookies.
	// should probably be changed in production, but not random
	// so that cookies stay valid across nodes/app restarts.
	CookieCodec = securecookie.New([]byte("12345678901234567890123456789012"), nil)
	// how long to keep a stream online after the broadcaster has disconnected.
	// if the stream does not resume within this time, all clients get dropped.
	StreamKeepAlive = 10 * time.Second
)

func CreateDatabase(iface string) Database {
	d, err := NewSQLDatabase(iface, "sqlite3", "development.db")
	if err != nil {
		panic(err.Error())
	}
	return d
}

func init() {
	flag.StringVar(&DefaultInterface, "iface", ":8000", "[network]:port to bind on")
}
