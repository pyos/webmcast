package main

import (
	"database/sql"
	"sync"
)

type sqlDAO struct {
	sql.DB
	// The string written to `streams.server` of streams owned by this server.
	localhost string
	// Security tokens of active streams owned by this server.
	// Some broadcasting software (*cough* gstreamer *cough*) wraps each frame
	// in a separate request, which may or may not overload the database...
	streamTokenLock sync.RWMutex
	streamTokens    map[string]string
}

const sqlSchema = `
create table if not exists users (
    id           integer      not null primary key,
    actoken      varchar(64),
    sectoken     varchar(64)  not null,
    name         varchar(256) not null,
    login        varchar(256) not null,
    email        varchar(256) not null,
    pwhash       varchar(256) not null,
    about        text         not null default "",
    unique(login), unique(email)
);

create table if not exists streams (
    id         integer      not null primary key,
    user       integer      not null,
    video      boolean      not null default 1,
    audio      boolean      not null default 1,
    width      integer      not null default 0,
    height     integer      not null default 0,
    name       varchar(256) not null default "",
    server     varchar(128)
);

create table if not exists panels (
    id        integer      not null primary key,
    stream    integer      not null,
    text      text         not null,
    image     varchar(256) not null default ""
);`

func NewSQLDatabase(localhost string, driver string, server string) (Database, error) {
	db, err := sql.Open(driver, server)
	if err == nil {
		wrapped := &sqlDAO{*db, localhost, sync.RWMutex{}, make(map[string]string)}
		if _, err = wrapped.Exec(sqlSchema); err == nil {
			return wrapped, nil
		}
		wrapped.Close()
	}
	return nil, err
}

func (d *sqlDAO) userExists(login string, email string) bool {
	var i int
	return d.QueryRow("select 1 from users where login = ? or email = ?", login, email).Scan(&i) != sql.ErrNoRows
}

func (d *sqlDAO) NewUser(login string, email string, password []byte) (*UserData, error) {
	if err := ValidateUsername(login); err != nil {
		return nil, err
	}
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	actoken := makeToken(tokenLength)
	sectoken := makeToken(tokenLength)
	r, err := d.Exec(
		"insert into users(actoken, sectoken, name, login, email, pwhash) values(?, ?, ?, ?, ?, ?)",
		actoken, sectoken, login, login, email, hash,
	)
	if err != nil {
		if d.userExists(login, email) {
			return nil, ErrUserNotUnique
		}
		return nil, err
	}

	uid, err := r.LastInsertId()
	if err == nil {
		_, err = d.Exec("insert into streams(user) values(?)", uid)
	}
	return &UserData{uid, login, email, login, hash, "", false, actoken, sectoken}, err
}

func (d *sqlDAO) ActivateUser(id int64, token string) error {
	r, err := d.Exec("update users set actoken = NULL where id = ? and actoken = ?", id, token)
	if err != nil {
		return err
	}
	changed, err := r.RowsAffected()
	if err == nil && changed != 1 {
		return ErrInvalidToken
	}
	return err
}

func (d *sqlDAO) GetUserID(login string, password []byte) (int64, error) {
	var u UserData
	err := d.QueryRow("select id, pwhash from users where login = ?", login).Scan(&u.ID, &u.PwHash)
	if err == sql.ErrNoRows {
		err = ErrUserNotExist
	} else if err == nil {
		err = u.CheckPassword(password)
	}
	return u.ID, err
}

func (d *sqlDAO) GetUserFull(id int64) (*UserData, error) {
	var actoken sql.NullString
	u := UserData{ID: id}
	err := d.QueryRow(
		"select name, login, email, pwhash, about, actoken, sectoken from users where id = ?", id,
	).Scan(
		&u.Name, &u.Login, &u.Email, &u.PwHash, &u.About, &actoken, &u.StreamToken,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotExist
	}
	if u.Activated = !actoken.Valid; actoken.Valid {
		u.ActivationToken = actoken.String
	}
	return &u, err
}

func (d *sqlDAO) SetUserData(id int64, name string, login string, email string, about string, password []byte) (string, error) {
	token := ""
	query := "update users set "
	params := make([]interface{}, 0, 7)

	if name != "" {
		query += "name = ?, "
		params = append(params, name)
	}

	if login != "" {
		if err := ValidateUsername(login); err != nil {
			return "", err
		}
		query += "login = ?, "
		params = append(params, login)
	}

	if email != "" {
		if err := ValidateEmail(email); err != nil {
			return "", err
		}
		token = makeToken(tokenLength)
		query += "actoken = ?, email = ?, "
		params = append(params, token, email)
	}

	if len(password) != 0 {
		hash, err := hashPassword(password)
		if err != nil {
			return "", err
		}
		query += "password = ?, "
		params = append(params, hash)
	}

	query += "about = ? where id = ? and not exists(select 1 from streams where user = users.id and server is not null)"
	params = append(params, about, id)

	r, err := d.Exec(query, params...)
	if err != nil {
		if (name != "" || email != "") && d.userExists(name, email) {
			return "", ErrUserNotUnique
		}
		return "", err
	}
	rows, err := r.RowsAffected()
	if err == nil && rows != 1 {
		return "", ErrStreamActive
	}
	return token, err
}

func errOf(_ interface{}, err error) error {
	return err
}

func (d *sqlDAO) NewStreamToken(id int64) error {
	// TODO invalidate token cache on all nodes
	//      damn, it appears I ran into the most difficult problem...
	return errOf(d.Exec("update users set sectoken = ? where id = ?", makeToken(tokenLength), id))
}

func (d *sqlDAO) SetStreamName(id int64, name string) error {
	return errOf(d.Exec("update streams set name = ? where user = ?", name, id))
}

func (d *sqlDAO) AddStreamPanel(id int64, text string) error {
	return errOf(d.Exec("insert into panels(stream, text) select id, ? from streams where user = ?", text, id))
}

func (d *sqlDAO) SetStreamPanel(id int64, n int64, text string) error {
	return errOf(d.Exec("update panels set text = ? where id in (select id from panels where stream in (select id from streams where user = ?) limit 1 offset ?)", text, id, n))
}

func (d *sqlDAO) DelStreamPanel(id int64, n int64) error {
	return errOf(d.Exec("delete from panels where id in (select id from panels where stream in (select id from streams where user = ?) limit 1 offset ?)", id, n))
}

func (d *sqlDAO) StartStream(id string, token string) error {
	d.streamTokenLock.RLock()
	if expect, ok := d.streamTokens[id]; ok {
		d.streamTokenLock.RUnlock()
		if expect != token {
			return ErrInvalidToken
		}
		return nil
	}
	d.streamTokenLock.RUnlock()

	_, err := d.Exec("update streams set server = ? where server is null and user in (select id from users where login = ? and actoken is null and sectoken = ?)", d.localhost, id, token)
	if err != nil {
		return err
	}

	var expect string
	var server sql.NullString
	var activated = true

	err = d.QueryRow("select sectoken, server, actoken is null from users join streams on users.id = streams.user where users.login = ?", id).Scan(&expect, &server, &activated)
	if err == sql.ErrNoRows || !activated {
		return ErrStreamNotExist
	}
	if err != nil {
		return err
	}
	if expect != token {
		return ErrInvalidToken
	}
	if !server.Valid || server.String != d.localhost {
		return ErrStreamNotHere
	}
	d.streamTokenLock.Lock()
	d.streamTokens[id] = expect
	d.streamTokenLock.Unlock()
	return nil
}

func (d *sqlDAO) StopStream(id string) error {
	d.streamTokenLock.Lock()
	delete(d.streamTokens, id)
	d.streamTokenLock.Unlock()
	_, err := d.Exec("update streams set server = null where user in (select id from users where login = ?)", id)
	return err
}

func (d *sqlDAO) GetStreamServer(id string) (string, error) {
	d.streamTokenLock.RLock()
	if _, ok := d.streamTokens[id]; ok {
		d.streamTokenLock.RUnlock()
		return d.localhost, nil
	}
	d.streamTokenLock.RUnlock()

	var intId int64
	var server sql.NullString
	err := d.QueryRow("select id, server from streams where user in (select id from users where login = ?)", id).Scan(&intId, &server)
	if err == sql.ErrNoRows {
		return "", ErrStreamNotExist
	}
	if err != nil {
		return "", err
	}
	if !server.Valid {
		return "", ErrStreamOffline
	}
	if server.String != d.localhost {
		return server.String, ErrStreamNotHere
	}
	if _, err = d.Exec("update streams set server = null where id = ?", intId); err != nil {
		return "", err
	}
	return "", ErrStreamOffline
}

func (d *sqlDAO) GetStreamMetadata(id string) (*StreamMetadata, error) {
	var intId int
	var server sql.NullString
	meta := StreamMetadata{}
	err := d.QueryRow(
		"select users.name, about, email, streams.name, server, video, audio, width, height, streams.id from users join streams on users.id = streams.user where login = ?", id,
	).Scan(
		&meta.UserName, &meta.UserAbout, &meta.Email, &meta.Name, &server, &meta.HasVideo, &meta.HasAudio, &meta.Width, &meta.Height, &intId,
	)
	if err == sql.ErrNoRows {
		return nil, ErrStreamNotExist
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.Query("select text, image from panels where stream = ?", intId)
	if err == nil {
		var panel StreamMetadataPanel
		for rows.Next() {
			if err = rows.Scan(&panel.Text, &panel.Image); err != nil {
				break
			}
			meta.Panels = append(meta.Panels, panel)
		}
		if err = rows.Err(); err == nil && !server.Valid {
			err = ErrStreamOffline
		}
		rows.Close()
		meta.Server = server.String
	}
	return &meta, err
}

func (d *sqlDAO) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	return errOf(d.Exec(
		"update streams set video = ?, audio = ?, width = ?, height = ? where user in (select id from users where login = ?)",
		info.HasVideo, info.HasAudio, info.Width, info.Height, id,
	))
}
