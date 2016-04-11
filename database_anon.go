package main

type AnonDatabase struct {
	active map[string]int
}

func NewAnonDatabase() Database {
	return &AnonDatabase{make(map[string]int)}
}

func (d *AnonDatabase) NewUser(name string, email string, password []byte) (*UserMetadata, error) {
	return nil, ErrNotSupported
}

func (d *AnonDatabase) GetUserID(email string, password []byte) (int64, error) {
	return 0, ErrNotSupported
}

func (d *AnonDatabase) GetUserFull(email string, password []byte) (*UserMetadata, error) {
	return nil, ErrNotSupported
}

func (d *AnonDatabase) ActivateUser(id int64, token string) error {
	return ErrNotSupported
}

func (d *AnonDatabase) SetUserName(id int64, name string, displayName string) error {
	return ErrNotSupported
}

func (d *AnonDatabase) SetUserEmail(id int64, email string) (string, error) {
	return "", ErrNotSupported
}

func (d *AnonDatabase) SetUserAbout(id int64, about string) error {
	return ErrNotSupported
}

func (d *AnonDatabase) SetUserPassword(id int64, password []byte) error {
	return ErrNotSupported
}

func (d *AnonDatabase) SetStreamName(id int64, name string) error {
	return ErrNotSupported
}

func (d *AnonDatabase) SetStreamAbout(id int64, about string) error {
	return ErrNotSupported
}

func (d *AnonDatabase) StartStream(user string, token string) error {
	d.active[user] = 1
	return nil
}

func (d *AnonDatabase) StopStream(user string) error {
	delete(d.active, user)
	return nil
}

func (d *AnonDatabase) GetStreamServer(user string) (string, error) {
	if _, ok := d.active[user]; ok {
		return "", nil
	}
	return "", ErrStreamNotExist
}

func (d *AnonDatabase) GetStreamMetadata(user string) (*StreamMetadata, error) {
	return &StreamMetadata{}, nil
}
