package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"math/rand"
	"strings"
	"time"
)

var (
	ErrNotSupported    = errors.New("Unsupported operation.")
	ErrInvalidEmail    = errors.New("Email does not look correct.")
	ErrInvalidPassword = errors.New("Cannot choose this password.")
	ErrInvalidToken    = errors.New("Invalid token.")
	ErrInvalidUsername = errors.New("Cannot choose this username.")
	ErrUserNotExist    = errors.New("Invalid username/password.")
	ErrUserNotUnique   = errors.New("This name/email is already taken.")
	ErrStreamActive    = errors.New("Can't do that while a stream is active.")
	ErrStreamNotExist  = errors.New("Unknown stream.")
	ErrStreamNotHere   = errors.New("Stream is online on another server.")
	ErrStreamOffline   = errors.New("Stream is offline.")
)

const (
	tokenAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	tokenLength   = 30
)

func makeToken(length int) string {
	xs := make([]byte, length)
	for i := 0; i < length; i++ {
		xs[i] = tokenAlphabet[rand.Intn(len(tokenAlphabet))]
	}
	return string(xs)
}

type UserData struct {
	ID              int64
	Login           string
	Email           string
	Name            string
	PwHash          []byte
	About           string
	Activated       bool
	ActivationToken string
	StreamToken     string
}

type StreamMetadata struct {
	UserName  string
	UserAbout string
	Name      string
	Email     string
	Server    string
	OwnerID   int64
	NSFW      bool
	Panels    []StreamMetadataPanel
	StreamTrackInfo
}

type StreamMetadataPanel struct {
	Text    string
	Image   string
	Created time.Time
}

type StreamTrackInfo struct {
	HasVideo bool
	HasAudio bool
	Width    uint // Dimensions of the video track that came last in the `Tracks` tag.
	Height   uint // Hopefully, there's only one video track in the file.
}

type FileSize int64

const (
	_            = iota
	KiB FileSize = 1 << (10 * iota)
	MiB
	GiB
)

type StreamHistory struct {
	UserName   string
	UserAbout  string
	Email      string
	SpaceUsed  FileSize
	SpaceLimit FileSize
	OwnerID    int64
	Recordings []StreamHistoryEntry
}

type StreamHistoryEntry struct {
	ID        int64
	Name      string
	Server    string
	Path      string
	Space     FileSize
	Timestamp time.Time
}

type StreamRecording struct {
	StreamMetadata
	Path      string
	Space     FileSize
	Timestamp time.Time
}

func hashPassword(password []byte) ([]byte, error) {
	if len(password) < 4 || len(password) > 128 {
		return []byte{}, ErrInvalidPassword
	}
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}

func (u *UserData) CheckPassword(password []byte) error {
	err := bcrypt.CompareHashAndPassword(u.PwHash, password)
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return ErrUserNotExist
	}
	return err
}

func gravatarURL(email string, size int) string {
	hash := md5.Sum([]byte(strings.ToLower(email)))
	hexhash := hex.EncodeToString(hash[:])
	return fmt.Sprintf("//www.gravatar.com/avatar/%s?s=%d", hexhash, size)
}

func (u *UserData) Avatar(size int) string {
	return gravatarURL(u.Email, size)
}

func (s *StreamMetadata) Avatar(size int) string {
	return gravatarURL(s.Email, size)
}

func (h *StreamHistory) Avatar(size int) string {
	return gravatarURL(h.Email, size)
}

func (s FileSize) RatioOf(t FileSize) float32 {
	if t == 0 {
		return 1
	}
	return float32(s) / float32(t)
}

func (s FileSize) String() string {
	switch {
	case s >= GiB:
		return fmt.Sprintf("%.2f MiB", float32(s)/float32(GiB))
	case s >= MiB:
		return fmt.Sprintf("%.2f MiB", float32(s)/float32(MiB))
	case s >= KiB:
		return fmt.Sprintf("%.2f KiB", float32(s)/float32(KiB))
	}
	return fmt.Sprintf("%d bytes", s)
}

type Database interface {
	Close() error
	NewUser(login string, email string, password []byte) (*UserData, error)
	ResetUser(login string, orEmail string) (uid int64, rstoken string, e error)
	ResetUserStep2(id int64, token string, password []byte) error
	ActivateUser(id int64, token string) error
	GetUserID(login string, password []byte) (int64, error)
	GetUserFull(id int64) (*UserData, error)
	// v--- can assume existence of user with given id
	SetUserData(id int64, name string, login string, email string, about string, password []byte) (actoken string, e error)
	NewStreamToken(id int64) error
	SetStreamName(id int64, name string, nsfw bool) error
	AddStreamPanel(id int64, text string) error
	SetStreamPanel(id int64, n int64, text string) error
	DelStreamPanel(id int64, n int64) error
	// v--- must accept string ids to be usable from broadcasting nodes (which don't deal in users)
	StartStream(id string, token string) error
	StopStream(id string) error
	GetStreamServer(id string) (string, error)
	GetStreamMetadata(id string) (*StreamMetadata, error)
	SetStreamTrackInfo(id string, info *StreamTrackInfo) error
	GetRecordings(id string) (*StreamHistory, error)
	GetRecording(id string, recid int64) (*StreamRecording, error)
	// TODO allow removing old recordings
	// TODO think of how to implement this ---v
	StartRecording(id string, filename string) (recid int64, sizeLimit int64, e error)
	StopRecording(id string, recid int64, size int64) error
}
