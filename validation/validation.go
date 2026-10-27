package validation

import (
	"errors"
	"regexp"
	"strings"
)

const MaxChildNameLength = 30

var childNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z' \-]*$`)

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
