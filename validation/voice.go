package validation

import (
	"errors"
	"strings"
)

var VoiceTones = []string{"Warm", "Brave", "Gentle", "Character", "Friendly", "Neutral"}

func VoiceTone(raw string) (string, error) {
	tone := strings.TrimSpace(raw)
	for _, known := range VoiceTones {
		if tone == known {
			return tone, nil
		}
	}
	return "", errors.New("voice_tone must be one of: " + strings.Join(VoiceTones, ", "))
}
