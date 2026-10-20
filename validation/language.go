package validation

import (
	"errors"
	"strings"
)

const StoryLanguage = "Basic English"

func Language(raw string) (string, error) {
	language := strings.TrimSpace(raw)
	switch language {
	case "", StoryLanguage:
		return StoryLanguage, nil
	default:
		return "", errors.New("language must be " + StoryLanguage)
	}
}
