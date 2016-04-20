package main

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
)

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

func NewSQLDatabase(localhost string, driver string, server string) (Database, error) {
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

	return &UserMetadata{
		UserShortData{uid, name, email, name}, "", false, activationToken, streamToken,
	}, nil
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

func (d *SQLDatabase) GetUserID(name string, password []byte) (int64, error) {
	var id int64
	var hash []byte
	err := d.QueryRow(`select id, password from users where name = ?`, name).Scan(&id, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrUserNotExist
		}
		return 0, err
	}
	err = bcrypt.CompareHashAndPassword(hash, password)
	if err == bcrypt.ErrMismatchedHashAndPassword {
		err = ErrUserNotExist
	}
	return id, err
}

func (d *SQLDatabase) GetUserShort(id int64) (*UserShortData, error) {
	meta := UserShortData{ID: id}
	err := d.QueryRow(`select name, display_name, email from users where users.id = ?`, id).Scan(
		&meta.Login, &meta.Name, &meta.Email,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotExist
	}
	return &meta, err
}

func (d *SQLDatabase) GetUserFull(id int64) (*UserMetadata, error) {
	meta := UserMetadata{UserShortData: UserShortData{ID: id}}
	err := d.QueryRow(
		`select users.name, email, display_name, users.about,
		        activated, activation_token, streams.token from users, streams
		 where users.id = ? and streams.id = users.id`,
		id,
	).Scan(
		&meta.Login, &meta.Email, &meta.Name, &meta.About,
		&meta.Activated, &meta.ActivationToken, &meta.StreamToken,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotExist
	}
	return &meta, err
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
	var server sql.NullString
	meta := StreamMetadata{}
	err := d.QueryRow(
		`select display_name, users.about, email, streams.name, streams.about, streams.server
		 from   users, streams
		 where  users.name = ? and streams.id = users.id`,
		user,
	).Scan(&meta.UserName, &meta.UserAbout, &meta.Email, &meta.Name, &meta.About, &server)
	if err == sql.ErrNoRows {
		return nil, ErrStreamNotExist
	}
	if err != nil {
		return nil, err
	}
	meta.Server = server.String
	if !server.Valid {
		return &meta, ErrStreamOffline
	}
	return &meta, nil
}
