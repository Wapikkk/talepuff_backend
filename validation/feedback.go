package validation

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MinFeedbackLength = 5
	MaxFeedbackLength = 1000
	maxAppVersionLen  = 32
)

var (
	feedbackCategories = map[string]bool{"bug": true, "idea": true, "other": true}
	appVersionPattern  = regexp.MustCompile(`^[0-9A-Za-z.+_-]+$`)
)

func FeedbackCategory(raw string) (string, error) {
	category := strings.ToLower(strings.TrimSpace(raw))
	if !feedbackCategories[category] {
		return "", errors.New("category must be one of: bug, idea, other")
	}
	return category, nil
}

func FeedbackMessage(raw string) (string, error) {
	message := strings.TrimSpace(raw)
	length := utf8.RuneCountInString(message)
	switch {
	case length < MinFeedbackLength:
		return "", errors.New("message is too short")
	case length > MaxFeedbackLength:
		return "", errors.New("message is too long")
	}
	for _, r := range message {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return "", errors.New("message contains invalid characters")
		}
	}
	return message, nil
}

func AppVersion(raw string) string {
	version := strings.TrimSpace(raw)
	if version == "" || len(version) > maxAppVersionLen || !appVersionPattern.MatchString(version) {
		return ""
	}
	return version
}
