package main

type AnonDatabase int

func NewAnonDatabase() Database {
	return AnonDatabase(0)
}

func (d AnonDatabase) Close() error {
	return nil
}

func (d AnonDatabase) NewUser(name string, email string, password []byte) (*UserMetadata, error) {
	return nil, ErrNotSupported
}

func (d AnonDatabase) GetUserID(name string, password []byte) (int64, error) {
	return 0, ErrUserNotExist
}

func (d AnonDatabase) GetUserShort(id int64) (*UserShortData, error) {
	return nil, ErrUserNotExist
}

func (d AnonDatabase) GetUserFull(id int64) (*UserMetadata, error) {
	return nil, ErrUserNotExist
}

func (d AnonDatabase) ActivateUser(id int64, token string) error {
	return ErrUserNotExist
}

func (d AnonDatabase) SetUserMetadata(id int64, name string, displayName string, email string, about string, password []byte) (string, error) {
	return "", ErrNotSupported
}

func (d AnonDatabase) SetStreamName(id int64, name string) error {
	return ErrNotSupported
}

func (d AnonDatabase) SetStreamAbout(id int64, about string) error {
	return ErrNotSupported
}

func (d AnonDatabase) NewStreamToken(id int64) error {
	return ErrNotSupported
}

func (d AnonDatabase) StartStream(user string, token string) error {
	return nil
}

func (d AnonDatabase) StopStream(user string) error {
	return nil
}

func (d AnonDatabase) GetStreamServer(user string) (string, error) {
	return "", ErrStreamNotExist
}

func (d AnonDatabase) GetStreamMetadata(user string) (*StreamMetadata, error) {
	return &StreamMetadata{}, nil
}
