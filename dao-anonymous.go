package main

import "sync"

type anonymousDAO struct {
	active map[string]*StreamMetadata
	*sync.RWMutex
}

func NewAnonDatabase() Database {
	return anonymousDAO{make(map[string]*StreamMetadata), new(sync.RWMutex)}
}

func (d anonymousDAO) Close() error {
	return nil
}

func (d anonymousDAO) NewUser(login string, email string, password []byte) (*UserData, error) {
	return nil, ErrNotSupported
}

func (d anonymousDAO) ActivateUser(id int64, token string) error {
	return ErrUserNotExist
}

func (d anonymousDAO) GetUserID(login string, password []byte) (int64, error) {
	return 0, ErrUserNotExist
}

func (d anonymousDAO) GetUserFull(id int64) (*UserData, error) {
	return nil, ErrUserNotExist
}

func (d anonymousDAO) SetUserData(id int64, name string, login string, email string, about string, password []byte) (string, error) {
	return "", ErrNotSupported
}

func (d anonymousDAO) NewStreamToken(id int64) error {
	return ErrNotSupported
}

func (d anonymousDAO) SetStreamName(id int64, name string) error {
	return ErrNotSupported
}

func (d anonymousDAO) AddStreamPanel(id int64, text string) error {
	return ErrNotSupported
}

func (d anonymousDAO) SetStreamPanel(id int64, n int64, text string) error {
	return ErrNotSupported
}

func (d anonymousDAO) DelStreamPanel(id int64, n int64) error {
	return ErrNotSupported
}

func (d anonymousDAO) StartStream(id string, token string) error {
	d.Lock()
	if _, ok := d.active[id]; !ok {
		d.active[id] = &StreamMetadata{StreamTrackInfo: StreamTrackInfo{HasVideo: true, HasAudio: true}}
	}
	d.Unlock()
	return nil
}

func (d anonymousDAO) StopStream(id string) error {
	d.Lock()
	delete(d.active, id)
	d.Unlock()
	return nil
}

func (d anonymousDAO) GetStreamServer(id string) (string, error) {
	d.RLock()
	if info, ok := d.active[id]; ok {
		d.RUnlock()
		return info.Server, nil
	}
	d.RUnlock()
	return "", ErrStreamNotExist
}

func (d anonymousDAO) GetStreamMetadata(id string) (*StreamMetadata, error) {
	d.RLock()
	if info, ok := d.active[id]; ok {
		d.RUnlock()
		return info, nil
	}
	d.RUnlock()
	return nil, ErrStreamNotExist
}

func (d anonymousDAO) SetStreamTrackInfo(id string, info *StreamTrackInfo) error {
	d.RLock()
	if item, ok := d.active[id]; ok {
		item.StreamTrackInfo = *info
		d.RUnlock()
		return nil
	}
	d.RUnlock()
	return ErrStreamNotExist
}
