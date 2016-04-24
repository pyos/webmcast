package database

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"math/rand"
	"strings"
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

type UserShortData struct {
	ID     int64
	Login  string
	Email  string
	Name   string
	PwHash []byte
}

type UserMetadata struct {
	UserShortData
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
	About     string
	Server    string
	StreamTrackInfo
}

type StreamTrackInfo struct {
	HasVideo bool
	HasAudio bool
	Width    uint // Dimensions of the video track that came last in the `Tracks` tag.
	Height   uint // Hopefully, there's only one video track in the file.
}

func hashPassword(password []byte) ([]byte, error) {
	if len(password) < 4 || len(password) > 128 {
		return []byte{}, ErrInvalidPassword
	}
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}

func (u *UserShortData) CheckPassword(password []byte) error {
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

func (u *UserShortData) GravatarURL(size int) string {
	return gravatarURL(u.Email, size)
}

func (s *StreamMetadata) GravatarURL(size int) string {
	return gravatarURL(s.Email, size)
}

type Interface interface {
	Close() error
	// Create a new user entry. Display name = name, activation token is generated randomly.
	NewUser(name string, email string, password []byte) (*UserMetadata, error)
	// Authenticate a user.
	GetUserID(name string, password []byte) (int64, error)
	GetUserShort(id int64) (*UserShortData, error)
	GetUserFull(id int64) (*UserMetadata, error)
	// TODO something for password recovery.
	// Allow a user to create streams.
	ActivateUser(id int64, token string) error
	NewStreamToken(id int64) error
	// An empty string in any field keeps the old value. Except for `about`,
	// which is set to an empty string. Changing the email address resets activation
	// status, in which case a new activation token is returned.
	SetUserMetadata(id int64, name string, displayName string, email string, about string, password []byte) (string, error)
	StartStream(id string, token string) error
	SetStreamName(id string, name string) error
	SetStreamAbout(id string, about string) error
	SetStreamTrackInfo(id string, info *StreamTrackInfo) error
	GetStreamServer(user string) (string, error)
	GetStreamMetadata(user string) (*StreamMetadata, error)
	StopStream(id string) error
}
