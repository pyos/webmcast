package common

import (
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	_ "github.com/mattn/go-sqlite3"
)

type Context struct {
	Database
	// the key used to sign client-side secure session cookies.
	// should probably be changed in production, but not random
	// so that cookies stay valid across nodes/app restarts.
	SecureKey   []byte
	cookieCodec *securecookie.SecureCookie
	// how long to keep a stream online after the broadcaster has disconnected.
	// if the stream does not resume within this time, all clients get dropped.
	StreamKeepAlive time.Duration
}

func (c *Context) GetAuthInfo(r *http.Request) (*UserData, error) {
	if c.cookieCodec == nil {
		c.cookieCodec = securecookie.New(c.SecureKey, nil)
	}
	var uid int64
	if cookie, err := r.Cookie("uid"); err == nil {
		if err = c.cookieCodec.Decode("uid", cookie.Value, &uid); err == nil {
			return c.GetUserFull(uid)
		}
	}
	return nil, ErrUserNotExist
}

func (c *Context) SetAuthInfo(w http.ResponseWriter, id int64) error {
	if id == -1 {
		http.SetCookie(w, &http.Cookie{Name: "uid", Value: "", Path: "/", MaxAge: 0})
	} else {
		if c.cookieCodec == nil {
			c.cookieCodec = securecookie.New(c.SecureKey, nil)
		}
		enc, err := c.cookieCodec.Encode("uid", id)
		if err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{
			Name: "uid", Value: enc, Path: "/", HttpOnly: true, MaxAge: 31536000,
		})
	}
	return nil
}
