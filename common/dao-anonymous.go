package common

import "sync"

type anonymous struct {
	active map[string]*StreamMetadata
	*sync.RWMutex
}

func NewAnonDatabase() Database {
	return anonymous{make(map[string]*StreamMetadata), new(sync.RWMutex)}
}

func (d anonymous) Close() error {
	return nil
}

func (d anonymous) NewUser(login string, email string, password []byte) (*UserData, error) {
	return nil, ErrNotSupported
}

func (d anonymous) ActivateUser(id int64, token string) error {
	return ErrUserNotExist
}

func (d anonymous) GetUserID(login string, password []byte) (int64, error) {
	return 0, ErrUserNotExist
}

func (d anonymous) GetUserFull(id int64) (*UserData, error) {
	return nil, ErrUserNotExist
}

func (d anonymous) SetUserData(id int64, name string, login string, email string, about string, password []byte) (string, error) {
	return "", ErrNotSupported
}

func (d anonymous) NewStreamToken(id int64) error {
	return ErrNotSupported
}

func (d anonymous) SetStreamName(id int64, name string) error {
	return ErrNotSupported
}

func (d anonymous) AddStreamPanel(id int64, text string) error {
	return ErrNotSupported
}

func (d anonymous) SetStreamPanel(id int64, n int64, text string) error {
	return ErrNotSupported
}

func (d anonymous) DelStreamPanel(id int64, n int64) error {
	return ErrNotSupported
}

func (d anonymous) StartStream(id string, token string) error {
	d.Lock()
	if _, ok := d.active[id]; !ok {
		d.active[id] = &StreamMetadata{StreamTrackInfo: StreamTrackInfo{HasVideo: true, HasAudio: true}}
	}
	d.Unlock()
	return nil
}

func (d anonymous) StopStream(id string) error {
	d.Lock()
	delete(d.active, id)
	d.Unlock()
	return nil
}

func (d anonymous) GetStreamServer(id string) (string, error) {
	d.RLock()
	if info, ok := d.active[id]; ok {
		d.RUnlock()
		return info.Server, nil
	}
	d.RUnlock()
	return "", ErrStreamNotExist
}

func (d anonymous) GetStreamMetadata(id string) (*StreamMetadata, error) {
	d.RLock()
	if info, ok := d.active[id]; ok {
		d.RUnlock()
		return info, nil
	}
	d.RUnlock()
	return nil, ErrStreamNotExist
}

func (d anonymous) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	d.RLock()
	if item, ok := d.active[id]; ok {
		item.StreamTrackInfo = *info
		d.RUnlock()
		return nil
	}
	d.RUnlock()
	return ErrStreamNotExist
}
