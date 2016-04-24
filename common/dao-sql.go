package common

import "database/sql"

type sqlImpl struct {
	sql.DB
	// The string written to `streams.server` of streams owned by this server.
	localhost string
	// Security tokens of active streams owned by this server.
	// Some broadcasting software (*cough* gstreamer *cough*) wraps each frame
	// in a separate request, which may or may not overload the database...
	streamTokenCache map[string]string
}

const sqlSchema = `
create table if not exists users (
    id                integer      not null,
    activated         integer      not null default 0,
    activation_token  varchar(64)  not null,
    name              varchar(256) not null,
    email             varchar(256) not null,
    display_name      varchar(256) not null,
    about             text         not null default "",
    password          varchar(256) not null,
    stream_name       varchar(256) not null default "",
    stream_about      text         not null default "",
    stream_token      varchar(64)  not null,
    stream_server     varchar(128),
    stream_has_video  integer default 1,
    stream_has_audio  integer default 1,
    stream_w          integer default 0,
    stream_h          integer default 0,
    primary key (id), unique (name), unique (email)
);`

func NewSQLDatabase(localhost string, driver string, server string) (Database, error) {
	db, err := sql.Open(driver, server)
	if err == nil {
		wrapped := &sqlImpl{*db, localhost, make(map[string]string)}
		if _, err = wrapped.Exec(sqlSchema); err == nil {
			return wrapped, nil
		}
		wrapped.Close()
	}
	return nil, err
}

func (d *sqlImpl) userExists(name string, email string) bool {
	var i int
	err := d.QueryRow("select 1 from users where name = ? or email = ?", name, email).Scan(&i)
	return err != sql.ErrNoRows
}

func (d *sqlImpl) NewUser(name string, email string, password []byte) (*UserMetadata, error) {
	if err := ValidateUsername(name); err != nil {
		return nil, err
	}
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	activationToken := makeToken(tokenLength)
	streamToken := makeToken(tokenLength)
	r, err := d.Exec(
		`insert into users(activation_token, name, email, display_name, password, stream_token)
		 values (?, ?, ?, ?, ?, ?);`,
		activationToken, name, email, name, hash, streamToken,
	)
	if err != nil {
		if d.userExists(name, email) {
			return nil, ErrUserNotUnique
		}
		return nil, err
	}

	uid, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &UserMetadata{
		UserShortData{uid, name, email, name, hash}, "", false, activationToken, streamToken,
	}, nil
}

func (d *sqlImpl) ActivateUser(id int64, token string) error {
	r, err := d.Exec("update users set activated = 1 where id = ? and activation_token = ?", id, token)
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

func (d *sqlImpl) NewStreamToken(id int64) error {
	// TODO invalidate token cache on all nodes
	//      damn, it appears I ran into the most difficult problem...
	token := makeToken(tokenLength)
	_, err := d.Exec(`update users set stream_token = ? where id = ?`, token, id)
	return err
}

func (d *sqlImpl) GetUserID(name string, password []byte) (int64, error) {
	var meta UserShortData
	err := d.QueryRow(
		`select id, password from users where name = ?`, name,
	).Scan(&meta.ID, &meta.PwHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrUserNotExist
		}
		return 0, err
	}
	return meta.ID, meta.CheckPassword(password)
}

func (d *sqlImpl) GetUserShort(id int64) (*UserShortData, error) {
	meta := UserShortData{ID: id}
	err := d.QueryRow(
		`select name, password, display_name, email from users where users.id = ?`, id,
	).Scan(&meta.Login, &meta.PwHash, &meta.Name, &meta.Email)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotExist
	}
	return &meta, err
}

func (d *sqlImpl) GetUserFull(id int64) (*UserMetadata, error) {
	meta := UserMetadata{UserShortData: UserShortData{ID: id}}
	err := d.QueryRow(
		`select name, password, email, display_name, about, activated,
		        activation_token, stream_token from users where users.id = ?`,
		id,
	).Scan(
		&meta.Login, &meta.PwHash, &meta.Email, &meta.Name, &meta.About,
		&meta.Activated, &meta.ActivationToken, &meta.StreamToken,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotExist
	}
	return &meta, err
}

func (d *sqlImpl) SetUserMetadata(id int64, name string, displayName string, email string, about string, password []byte) (string, error) {
	token := ""
	query := "update users set "
	params := make([]interface{}, 0, 6)

	if name != "" {
		if err := ValidateUsername(name); err != nil {
			return "", err
		}
		query += "name = ?, "
		params = append(params, name)
	}

	if displayName != "" {
		query += "display_name = ?, "
		params = append(params, displayName)
	}

	if email != "" {
		if err := ValidateEmail(email); err != nil {
			return "", err
		}
		token = makeToken(tokenLength)
		query += "activated = 0, activation_token = ?, email = ?, "
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

	query += "about = ? where id = ? and stream_server is null"
	params = append(params, about, id)

	r, err := d.Exec(query, params...)
	if err != nil {
		if (name != "" || email != "") && d.userExists(name, email) {
			return "", ErrUserNotUnique
		}
		return "", err
	}
	rows, err := r.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows != 1 {
		return "", ErrStreamActive
	}
	return token, err
}

func (d *sqlImpl) StartStream(id string, token string) error {
	if expect, ok := d.streamTokenCache[id]; ok {
		if expect != token {
			return ErrInvalidToken
		}
		return nil
	}

	_, err := d.Exec(
		`update users set stream_server = ? where name = ? and activated = 1 and stream_server is null and stream_token = ?`,
		d.localhost, id, token,
	)
	if err != nil {
		return err
	}

	var expect string
	var server sql.NullString
	var activated = true
	err = d.QueryRow(`select stream_token, stream_server, activated from users where name = ?`, id).Scan(&expect, &server, &activated)
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
	d.streamTokenCache[id] = expect
	return nil
}

func (d *sqlImpl) SetStreamName(id string, name string) error {
	_, err := d.Exec(`update users set stream_name = ? where name = ?`, name, id)
	return err
}

func (d *sqlImpl) SetStreamAbout(id string, about string) error {
	_, err := d.Exec(`update users set stream_about = ? where name = ?`, about, id)
	return err
}

func (d *sqlImpl) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	_, err := d.Exec(
		`update users set stream_has_video = ?, stream_has_audio = ?, stream_w = ?, stream_h = ? where name = ?`,
		info.HasVideo, info.HasAudio, info.Width, info.Height, id,
	)
	return err
}

func (d *sqlImpl) GetStreamServer(id string) (string, error) {
	if _, ok := d.streamTokenCache[id]; ok {
		return d.localhost, nil
	}

	var server sql.NullString
	err := d.QueryRow(`select stream_server from users where name = ?`, id).Scan(&server)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrStreamNotExist
		}
		return "", err
	}
	if server.String == d.localhost {
		// it should have been offline??
		if _, err = d.Exec(`update users set stream_server = null where name = ?`, id); err != nil {
			return "", err
		}
		return "", ErrStreamOffline
	}
	if !server.Valid {
		return "", ErrStreamOffline
	}
	return server.String, ErrStreamNotHere
}

func (d *sqlImpl) GetStreamMetadata(id string) (*StreamMetadata, error) {
	var server sql.NullString
	meta := StreamMetadata{}
	err := d.QueryRow(
		`select display_name, about, email, stream_name, stream_about, stream_server,
		        stream_has_video, stream_has_audio, stream_w, stream_h
		 from users where name = ?`,
		id,
	).Scan(
		&meta.UserName, &meta.UserAbout, &meta.Email, &meta.Name, &meta.About, &server,
		&meta.HasVideo, &meta.HasAudio, &meta.Width, &meta.Height,
	)
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

func (d *sqlImpl) StopStream(id string) error {
	delete(d.streamTokenCache, id)
	_, err := d.Exec(`update users set stream_server = NULL where name = ?`, id)
	return err
}
