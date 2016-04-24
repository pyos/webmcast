package database

type anonymous int

func NewAnonDatabase() Interface {
	return anonymous(0)
}

func (d anonymous) Close() error {
	return nil
}

func (d anonymous) NewUser(name string, email string, password []byte) (*UserMetadata, error) {
	return nil, ErrNotSupported
}

func (d anonymous) GetUserID(name string, password []byte) (int64, error) {
	return 0, ErrUserNotExist
}

func (d anonymous) GetUserShort(id int64) (*UserShortData, error) {
	return nil, ErrUserNotExist
}

func (d anonymous) GetUserFull(id int64) (*UserMetadata, error) {
	return nil, ErrUserNotExist
}

func (d anonymous) ActivateUser(id int64, token string) error {
	return ErrUserNotExist
}

func (d anonymous) SetUserMetadata(id int64, name string, displayName string, email string, about string, password []byte) (string, error) {
	return "", ErrNotSupported
}

func (d anonymous) SetStreamName(id int64, name string) error {
	return ErrNotSupported
}

func (d anonymous) SetStreamAbout(id int64, about string) error {
	return ErrNotSupported
}

func (d anonymous) NewStreamToken(id int64) error {
	return ErrNotSupported
}

func (d anonymous) StartStream(user string, token string) error {
	return nil
}

func (d anonymous) StopStream(user string) error {
	return nil
}

func (d anonymous) GetStreamServer(user string) (string, error) {
	return "", ErrStreamNotExist
}

func (d anonymous) GetStreamMetadata(user string) (*StreamMetadata, error) {
	return &StreamMetadata{}, nil
}
