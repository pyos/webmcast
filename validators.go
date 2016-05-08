package main

import (
	"strings"
	"unicode"
)

func ValidateUsername(name string) error {
	if len(name) == 0 || len(name) > 32 {
		return ErrInvalidUsername
	}
	for _, c := range name {
		if !unicode.IsGraphic(c) {
			return ErrInvalidUsername
		}
	}
	return nil
}

func ValidateEmail(email string) error {
	if !strings.ContainsRune(email, '@') || len(email) < 3 || len(email) > 255 {
		return ErrInvalidEmail
	}
	return nil
}
