package database

type anonymous map[string]*StreamMetadata

func NewAnonDatabase() Interface {
	return make(anonymous)
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

func (d anonymous) NewStreamToken(id int64) error {
	return ErrNotSupported
}

func (d anonymous) SetUserMetadata(id int64, name string, displayName string, email string, about string, password []byte) (string, error) {
	return "", ErrNotSupported
}

func (d anonymous) StartStream(id string, token string) error {
	if _, ok := d[id]; !ok {
		d[id] = &StreamMetadata{StreamTrackInfo: StreamTrackInfo{HasVideo: true, HasAudio: true}}
	}
	return nil
}

func (d anonymous) SetStreamName(id string, name string) error {
	if info, ok := d[id]; ok {
		info.Name = name
		return nil
	}
	return ErrStreamNotExist
}

func (d anonymous) SetStreamAbout(id string, about string) error {
	if info, ok := d[id]; ok {
		info.About = about
		return nil
	}
	return ErrStreamNotExist
}

func (d anonymous) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	if item, ok := d[id]; ok {
		item.StreamTrackInfo = *info
		return nil
	}
	return ErrStreamNotExist
}

func (d anonymous) GetStreamServer(id string) (string, error) {
	if info, ok := d[id]; ok {
		return info.Server, nil
	}
	return "", ErrStreamNotExist
}

func (d anonymous) GetStreamMetadata(id string) (*StreamMetadata, error) {
	if info, ok := d[id]; ok {
		return info, nil
	}
	return nil, ErrStreamNotExist
}

func (d anonymous) StopStream(id string) error {
	delete(d, id)
	return nil
}
