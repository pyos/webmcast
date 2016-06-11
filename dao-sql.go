package main

import (
	"database/sql"
	"reflect"
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

	prepared struct {
		UserExists      *sql.Stmt "select 1 from users where login = ? or email = ?"
		NewUser         *sql.Stmt "insert into users(actoken, sectoken, name, login, email, pwhash) values(?, ?, ?, ?, ?, ?)"
		NewStream       *sql.Stmt "insert into streams(user) values(?)"
		ResetUser       *sql.Stmt "update users set rstoken = ? where id = ?"
		ResetUserStep2  *sql.Stmt "update users set pwhash = ? where id = ? and rstoken = ?"
		ActivateUser    *sql.Stmt "update users set actoken = NULL where id = ? and actoken = ?"
		GetUserID       *sql.Stmt "select id, pwhash from users where login = ?"
		GetUserByEither *sql.Stmt "select id from users where login = ? or email = ?"
		GetUserInfo     *sql.Stmt "select name, login, email, pwhash, about, actoken, sectoken from users where id = ?"
		GetStreamInfo   *sql.Stmt "select users.id, users.name, about, email, streams.name, server, video, audio, width, height, nsfw, streams.id from users join streams on users.id = streams.user where login = ?"
		SetStreamToken  *sql.Stmt "update users set sectoken = ? where id = ?"
		SetStreamName   *sql.Stmt "update streams set name = ?, nsfw = ? where user = ?"
		SetStreamTracks *sql.Stmt "update streams set video = ?, audio = ?, width = ?, height = ? where user in (select id from users where login = ?)"
		GetStreamPanels *sql.Stmt "select text, image, created from panels where stream = ?"
		AddStreamPanel  *sql.Stmt "insert into panels(stream, text) select id, ? from streams where user = ?"
		SetStreamPanel  *sql.Stmt "update panels set text = ? where id in (select id from panels where stream in (select id from streams where user = ?) limit 1 offset ?)"
		DelStreamPanel  *sql.Stmt "delete from panels where id in (select id from panels where stream in (select id from streams where user = ?) limit 1 offset ?)"
		GetStreamAuth   *sql.Stmt "select server, sectoken, actoken is null from users join streams on users.id = streams.user where users.login = ?"
		GetStreamServer *sql.Stmt "select server from streams where user in (select id from users where login = ?)"
		SetStreamServer *sql.Stmt "update streams set server = ? where server is null and user in (select id from users where login = ? and actoken is null and sectoken = ?)"
		DelStreamServer *sql.Stmt "update streams set server = null where user in (select id from users where login = ?)"
		GetRecordings1  *sql.Stmt "select id, name, about, email, space_total from users where login = ?"
		GetRecordings2  *sql.Stmt "select id, name, server, path, created, size from recordings where user = ? order by datetime(created) desc"
		GetRecordPanels *sql.Stmt "select text, image, created from panels where stream = ? and datetime(created) <= datetime(?)"
		GetRecording    *sql.Stmt "select users.id, users.name, about, email, recordings.name, server, video, audio, width, height, nsfw, path, size, created, stream from users join recordings on users.id = user where recordings.id = ?"
	}
}

const sqlSchema = `
create table if not exists users (
    id           integer      not null primary key,
    actoken      varchar(64),
    rstoken      varchar(64),
    sectoken     varchar(64)  not null,
    name         varchar(256) not null,
    login        varchar(256) not null,
    email        varchar(256) not null,
    pwhash       varchar(256) not null,
    about        text         not null default "",
    space_total  integer      not null default 0,
    unique(login), unique(email)
);

create table if not exists streams (
    id         integer      not null primary key,
    user       integer      not null,
    video      boolean      not null default 1,
    audio      boolean      not null default 1,
    nsfw       boolean      not null default 0,
    width      integer      not null default 0,
    height     integer      not null default 0,
    name       varchar(256) not null default "",
    server     varchar(128)
);

create table if not exists panels (
    id        integer      not null primary key,
    stream    integer      not null,
    text      text         not null,
    image     varchar(256) not null default "",
    created   datetime     not null default (datetime('now'))
);

create table if not exists recordings (
    id         integer      not null primary key,
    stream     integer      not null,
    user       integer      not null,
    video      boolean      not null default 1,
    audio      boolean      not null default 1,
    nsfw       boolean      not null default 0,
    width      integer      not null default 0,
    height     integer      not null default 0,
    name       varchar(256) not null default "",
    server     varchar(128) not null,
    path       varchar(256) not null,
    created    datetime     not null default (datetime('now')),
    size       integer      not null default 0
);`

func NewSQLDatabase(localhost string, driver string, server string) (Database, error) {
	db, err := sql.Open(driver, server)
	if err == nil {
		wrapped := &sqlDAO{DB: *db, localhost: localhost, streamTokens: make(map[string]string)}
		if err = wrapped.prepare(); err == nil {
			return wrapped, nil
		}
		wrapped.Close()
	}
	return nil, err
}

func (d *sqlDAO) prepare() error {
	if _, err := d.Exec(sqlSchema); err != nil {
		return err
	}
	t := reflect.TypeOf(&d.prepared).Elem()
	v := reflect.ValueOf(&d.prepared).Elem()
	for i := 0; i < t.NumField(); i++ {
		stmt, err := d.Prepare(string(t.Field(i).Tag))
		if err != nil {
			return err
		}
		v.Field(i).Set(reflect.ValueOf(stmt))
	}
	return nil
}

func (d *sqlDAO) userExists(login string, email string) bool {
	var i int
	return d.prepared.UserExists.QueryRow(login, email).Scan(&i) != sql.ErrNoRows
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
	r, err := d.prepared.NewUser.Exec(actoken, sectoken, login, login, email, hash)
	if err != nil {
		if d.userExists(login, email) {
			return nil, ErrUserNotUnique
		}
		return nil, err
	}

	uid, err := r.LastInsertId()
	if err == nil {
		_, err = d.prepared.NewStream.Exec(uid)
	}
	return &UserData{uid, login, email, login, hash, "", false, actoken, sectoken}, err
}

func (d *sqlDAO) ResetUser(login string, orEmail string) (uid int64, token string, err error) {
	token = makeToken(tokenLength)
	err = d.prepared.GetUserByEither.QueryRow(login, orEmail).Scan(&uid)
	if err == sql.ErrNoRows {
		err = ErrUserNotExist
	}
	if err == nil {
		_, err = d.prepared.ResetUser.Exec(token, uid)
	}
	return
}

func (d *sqlDAO) ResetUserStep2(id int64, token string, password []byte) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	r, err := d.prepared.ResetUserStep2.Exec(hash, id, token)
	if err != nil {
		return err
	}
	changed, err := r.RowsAffected()
	if err == nil && changed != 1 {
		return ErrUserNotExist
	}
	return err
}

func (d *sqlDAO) ActivateUser(id int64, token string) error {
	r, err := d.prepared.ActivateUser.Exec(id, token)
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
	err := d.prepared.GetUserID.QueryRow(login).Scan(&u.ID, &u.PwHash)
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
	err := d.prepared.GetUserInfo.QueryRow(id).Scan(
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
	offlineOnly := false

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
		offlineOnly = true
	}

	if email != "" {
		if err := ValidateEmail(email); err != nil {
			return "", err
		}
		token = makeToken(tokenLength)
		query += "actoken = ?, email = ?, "
		params = append(params, token, email)
		offlineOnly = true
	}

	if len(password) != 0 {
		hash, err := hashPassword(password)
		if err != nil {
			return "", err
		}
		query += "pwhash = ?, "
		params = append(params, hash)
	}

	query += "about = ? where id = ?"
	params = append(params, about, id)

	if offlineOnly {
		query += " and not exists(select 1 from streams where user = users.id and server is not null)"
	}
	r, err := d.Exec(query, params...)
	if err != nil {
		if (login != "" || email != "") && d.userExists(login, email) {
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
	return errOf(d.prepared.SetStreamToken.Exec(makeToken(tokenLength), id))
}

func (d *sqlDAO) SetStreamName(id int64, name string, nsfw bool) error {
	return errOf(d.prepared.SetStreamName.Exec(name, nsfw, id))
}

func (d *sqlDAO) AddStreamPanel(id int64, text string) error {
	return errOf(d.prepared.AddStreamPanel.Exec(text, id))
}

func (d *sqlDAO) SetStreamPanel(id int64, n int64, text string) error {
	return errOf(d.prepared.SetStreamPanel.Exec(text, id, n))
}

func (d *sqlDAO) DelStreamPanel(id int64, n int64) error {
	return errOf(d.prepared.DelStreamPanel.Exec(id, n))
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

	_, err := d.prepared.SetStreamServer.Exec(d.localhost, id, token)
	if err != nil {
		return err
	}

	var expect string
	var server sql.NullString
	var activated = true

	err = d.prepared.GetStreamAuth.QueryRow(id).Scan(&server, &expect, &activated)
	if err == sql.ErrNoRows {
		return ErrStreamNotExist
	}
	if err != nil {
		return err
	}
	if expect != token || !activated {
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
	_, err := d.prepared.DelStreamServer.Exec(id)
	return err
}

func (d *sqlDAO) GetStreamServer(id string) (string, error) {
	d.streamTokenLock.RLock()
	if _, ok := d.streamTokens[id]; ok {
		d.streamTokenLock.RUnlock()
		return d.localhost, nil
	}
	d.streamTokenLock.RUnlock()

	var server sql.NullString
	err := d.prepared.GetStreamServer.QueryRow(id).Scan(&server)
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
	if _, err = d.prepared.DelStreamServer.Exec(id); err != nil {
		return "", err
	}
	return "", ErrStreamOffline
}

func (d *sqlDAO) GetStreamMetadata(id string) (*StreamMetadata, error) {
	var intId int
	var server sql.NullString
	meta := StreamMetadata{}
	err := d.prepared.GetStreamInfo.QueryRow(id).Scan(
		&meta.OwnerID, &meta.UserName, &meta.UserAbout, &meta.Email, &meta.Name, &server,
		&meta.HasVideo, &meta.HasAudio, &meta.Width, &meta.Height, &meta.NSFW, &intId,
	)
	if err == sql.ErrNoRows {
		return nil, ErrStreamNotExist
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.prepared.GetStreamPanels.Query(intId)
	if err == nil {
		meta.Panels, err = d.loadPanelsFromRows(rows)
		if err == nil && !server.Valid {
			err = ErrStreamOffline
		}
		meta.Server = server.String
	}
	return &meta, err
}

func (d *sqlDAO) loadPanelsFromRows(rows *sql.Rows) ([]StreamMetadataPanel, error) {
	r := make([]StreamMetadataPanel, 0, 5)
	panel := StreamMetadataPanel{}
	for rows.Next() && rows.Scan(&panel.Text, &panel.Image, &panel.Created) == nil {
		r = append(r, panel)
	}
	rows.Close()
	return r, rows.Err()
}

func (d *sqlDAO) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	return errOf(d.prepared.SetStreamTracks.Exec(info.HasVideo, info.HasAudio, info.Width, info.Height, id))
}

func (d *sqlDAO) GetRecordings(id string) (*StreamHistory, error) {
	h := StreamHistory{}
	err := d.prepared.GetRecordings1.QueryRow(id).Scan(&h.OwnerID, &h.UserName, &h.UserAbout, &h.Email, &h.SpaceLimit)
	if err == sql.ErrNoRows {
		err = ErrStreamNotExist
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.prepared.GetRecordings2.Query(h.OwnerID)
	if err == nil {
		entry := StreamHistoryEntry{}
		for rows.Next() && rows.Scan(&entry.ID, &entry.Name, &entry.Server, &entry.Path, &entry.Timestamp, &entry.Space) == nil {
			h.SpaceUsed += entry.Space
			h.Recordings = append(h.Recordings, entry)
		}
		err = rows.Err()
		rows.Close()
	}
	return &h, err
}

func (d *sqlDAO) GetRecording(id string, recid int64) (*StreamRecording, error) {
	var intId int
	r := StreamRecording{}
	err := d.prepared.GetRecording.QueryRow(recid).Scan(
		&r.OwnerID, &r.UserName, &r.UserAbout, &r.Email, &r.Name, &r.Server, &r.HasVideo,
		&r.HasAudio, &r.Width, &r.Height, &r.NSFW, &r.Path, &r.Space, &r.Timestamp, &intId,
	)
	if err == sql.ErrNoRows {
		return nil, ErrStreamNotExist
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.prepared.GetRecordPanels.Query(intId, r.Timestamp)
	if err == nil {
		r.Panels, err = d.loadPanelsFromRows(rows)
	}
	return &r, err
}

func (d *sqlDAO) StartRecording(id string, filename string) (recid int64, sizeLimit int64, e error) {
	return 0, 0, nil
}

func (d *sqlDAO) StopRecording(id string, recid int64, size int64) error {
	return nil
}
