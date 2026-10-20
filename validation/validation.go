package validation

import (
	"errors"
	"regexp"
	"strings"
)

const MaxChildNameLength = 30

var (
	childNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z' \-]*$`)
	emailPattern     = regexp.MustCompile(`^[\w.+-]+@([\w-]+\.)+[\w-]{2,}$`)
)

func ChildName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", errors.New("child name is required")
	case len(name) > MaxChildNameLength:
		return "", errors.New("child name must be at most 30 characters")
	case !childNamePattern.MatchString(name):
		return "", errors.New("child name may only contain letters, spaces, apostrophes and hyphens, and must start with a letter")
	}
	return name, nil
}

func Email(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", errors.New("email is required")
	}
	if !emailPattern.MatchString(email) {
		return "", errors.New("email format is invalid")
	}
	return email, nil
}
