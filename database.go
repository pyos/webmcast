package main

import (
	"database/sql"
	"errors"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken   = errors.New("Invalid token.")
	ErrUserNotExist   = errors.New("Invalid username/password.")
	ErrUserNotUnique  = errors.New("This name/email is already taken.")
	ErrStreamActive   = errors.New("Can't do that while a stream is active.")
	ErrStreamNotExist = errors.New("Unknown stream.")
	ErrStreamNotHere  = errors.New("Stream is online on another server.")
	ErrStreamOffline  = errors.New("Stream is offline.")
)

type UserMetadata struct {
	ID              int64
	Login           string
	Email           string
	Name            string
	About           string
	Activated       bool
	ActivationToken string
	StreamToken     string
}

type StreamMetadata struct {
	UserName  string
	UserAbout string
	Name      string
	About     string
	Server    sql.NullString
}

type Database interface {
	// Create a new user entry. Display name = name, activation token is generated randomly.
	NewUser(name string, email string, password []byte) (*UserMetadata, error)
	// Authenticate a user. (The only way to retrieve a user ID, by design.)
	GetUserID(email string, password []byte) (int64, error)
	// A version of the above function that returns the rest of the data too.
	GetUserFull(email string, password []byte) (*UserMetadata, error)
	// Allow a user to create streams.
	ActivateUser(id int64, token string) error
	// Various setters. They're separate for efficiency; requests to modify
	// different fields are expected to be made via XHR separate from each other.
	SetUserName(id int64, name string, displayName string) error
	SetUserEmail(id int64, email string) (string, error) // returns new activation token
	SetUserAbout(id int64, about string) error
	SetUserPassword(id int64, password []byte) error
	// stream id = user id
	SetStreamName(id int64, name string) error
	SetStreamAbout(id int64, about string) error
	// Mark a stream as active on the current server.
	StartStream(user string, token string) error
	// Mark a stream as offline.
	StopStream(user string) error
	// Retrieve the string identifying the owner of the stream.
	// Clients talking to the wrong server may be redirected there, for example.
	// Unless the result is the current server, an ErrStreamNotHere is also returned.
	GetStreamServer(user string) (string, error)
	GetStreamMetadata(user string) (*StreamMetadata, error)
}

type SQLDatabase struct {
	sql.DB
	// The string written to `streams.server` of streams owned by this server.
	localhost string
	// Security tokens of active streams owned by this server.
	// Some broadcasting software (*cough* gstreamer *cough*) wraps each frame
	// in a separate request, which may or may not overload the database...
	streamTokenCache map[string]string
}

var SQLDatabaseSchema = `
create table if not exists users (
    id                integer  not null,
    activated         integer  not null,
    activation_token  text     not null,
    name              text     not null,
    email             text     not null,
    display_name      text     not null,
    about             text     not null,
    password          blob     not null,

    primary key (id), unique (name), unique (email)
);

create table if not exists streams (
    id      integer  not null,
    name    text     not null,
    about   text     not null,
    token   text     not null,
    server  text,

    primary key (id)
);`

func NewDatabase(localhost string, driver string, server string) (Database, error) {
	db, err := sql.Open(driver, server)
	if err != nil {
		return nil, err
	}
	wrapped := &SQLDatabase{*db, localhost, make(map[string]string)}
	if err = wrapped.Ping(); err != nil {
		wrapped.Close()
		return nil, err
	}
	if _, err = wrapped.Exec(SQLDatabaseSchema); err != nil {
		wrapped.Close()
		return nil, err
	}
	return wrapped, nil
}

func makeToken(length int) string {
	return "" // TODO
}

func (d *SQLDatabase) NewUser(name string, email string, password []byte) (*UserMetadata, error) {
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	activationToken := makeToken(20)
	streamToken := makeToken(20)
	r, err := d.Exec(
		`begin;
         insert into users   values (NULL, 0, ?, ?, ?, ?, "", ?);
         insert into streams values (NULL, "", "", ?, NULL);
         commit;`,
		activationToken, name, email, name, hash, streamToken,
	)
	if err != nil {
		var i int
		q := d.QueryRow(`select 1 from users where name = ? or email = ?`, name, email)
		if q.Scan(&i) != sql.ErrNoRows {
			return nil, ErrUserNotUnique
		}
		return nil, err
	}

	uid, err := r.LastInsertId()
	if err != nil {
		// FIXME uh...
		return nil, err
	}

	return &UserMetadata{uid, name, email, name, "", false, activationToken, streamToken}, nil
}

func (d *SQLDatabase) ActivateUser(id int64, token string) error {
	r, err := d.Exec(
		`update users set activated = 1 where id = ? and activation_token = ?`,
		id, token,
	)
	if err != nil {
		return err
	}

	changed, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if changed != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (d *SQLDatabase) GetUserID(email string, password []byte) (int64, error) {
	var id int64
	var hash []byte
	err := d.QueryRow(`select id, password from users where email = ?`, email).Scan(&id, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrUserNotExist
		}
		return 0, err
	}
	return id, bcrypt.CompareHashAndPassword(hash, password)
}

func (d *SQLDatabase) GetUserFull(email string, password []byte) (*UserMetadata, error) {
	var hash []byte
	meta := UserMetadata{}
	err := d.QueryRow(
		`select password, users.id, users.name, email, display_name, users.about,
                activated, activation_token, streams.token from users, streams
         where users.email = ? and streams.id = users.id`,
		email,
	).Scan(&hash, &meta.ID, &meta.Login, &meta.Email, &meta.Name,
		&meta.About, &meta.Activated, &meta.ActivationToken, &meta.StreamToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotExist
		}
		return nil, err
	}
	return &meta, bcrypt.CompareHashAndPassword(hash, password)
}

func (d *SQLDatabase) SetUserName(id int64, name string, displayName string) error {
	r, err := d.Exec(
		`update users set name = ?, display_name = ?
		    where id in (select id from streams where id = ? and server is null)`,
		name, displayName, id, id,
	)
	rows, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrStreamActive
	}
	return err
}

func (d *SQLDatabase) SetUserEmail(id int64, email string) (string, error) {
	token := makeToken(20)
	_, err := d.Exec(
		`update users set email = ?, activated = 0, activation_token = ? where id = ?`,
		email, token, id,
	)
	return token, err
}

func (d *SQLDatabase) SetUserAbout(id int64, about string) error {
	_, err := d.Exec(`update users set about = ? where id = ?`, about, id)
	return err
}

func (d *SQLDatabase) SetUserPassword(id int64, password []byte) error {
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err == nil {
		_, err = d.Exec(`update users set password = ? where id = ?`, hash, id)
	}
	return err
}

func (d *SQLDatabase) SetStreamName(id int64, name string) error {
	_, err := d.Exec(`update streams set name = ? where id = ?`, name, id)
	return err
}

func (d *SQLDatabase) SetStreamAbout(id int64, about string) error {
	_, err := d.Exec(`update streams set about = ? where id = ?`, about, id)
	return err
}

func (d *SQLDatabase) StartStream(user string, token string) error {
	if expect, ok := d.streamTokenCache[user]; ok {
		if expect != token {
			return ErrInvalidToken
		}
		return nil
	}

	var id int64
	var expect string
	var server sql.NullString

	if err := d.QueryRow(`select id from users where name = ?`, user).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return ErrStreamNotExist
		}
		return err
	}

	_, err := d.Exec(
		`update streams set server = ? where id = ? and server is null and token = ?`,
		d.localhost, id, token,
	)
	if err != nil {
		return err
	}

	err = d.QueryRow(`select token, server from streams where id = ?`, id).Scan(&expect, &server)
	if err != nil {
		return err
	}

	if expect != token {
		return ErrInvalidToken
	}
	if !server.Valid || server.String != d.localhost {
		return ErrStreamNotHere
	}
	d.streamTokenCache[user] = expect
	return nil
}

func (d *SQLDatabase) StopStream(user string) error {
	_, err := d.Exec(
		`update streams set server = NULL where id in (select id from users where name = ?)`,
		user,
	)
	if err != nil {
		return err
	}
	delete(d.streamTokenCache, user)
	return nil
}

func (d *SQLDatabase) GetStreamServer(user string) (string, error) {
	if _, ok := d.streamTokenCache[user]; ok {
		return d.localhost, nil
	}

	var server sql.NullString
	err := d.QueryRow(
		`select server from streams where id in (select id from users where name = ?)`,
		user,
	).Scan(&server)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrStreamNotExist
		}
		return "", err
	}
	if server.String == d.localhost {
		// it should have been offline??
		return "", ErrStreamActive
	}
	if !server.Valid {
		return "", ErrStreamOffline
	}
	return server.String, ErrStreamNotHere
}

func (d *SQLDatabase) GetStreamMetadata(user string) (*StreamMetadata, error) {
	meta := StreamMetadata{}
	err := d.QueryRow(
		`select users.display_name, users.about, streams.name, streams.about, streams.server
         from   users, streams
         where  users.name = ? and streams.id = users.id`,
		user,
	).Scan(&meta.UserName, &meta.UserAbout, &meta.Name, &meta.About, &meta.Server)
	if err == sql.ErrNoRows {
		return nil, ErrStreamNotExist
	}
	return &meta, err
}
