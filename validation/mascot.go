package validation

import (
	"errors"
	"strings"
)

var Mascots = []string{"Harimau", "OrangUtan", "Badak", "Burung"}

const DefaultMascot = "Harimau"

func Mascot(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	for _, mascot := range Mascots {
		if id == mascot {
			return id, nil
		}
	}
	return "", errors.New("mascot_id must be one of: " + strings.Join(Mascots, ", "))
}
