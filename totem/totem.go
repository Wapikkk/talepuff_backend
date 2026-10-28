// Package totem menghubungkan backend dengan totem (Pico) lewat MQTT.
// Kontrak pesan ada di totem_firmware/README.md.
package totem

import (
	"errors"
	"regexp"
	"time"
)

var ErrNotConfigured = errors.New("totem is not configured")
var ErrNotConnected = errors.New("not connected to the MQTT broker")

var deviceID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,31}$`)

// ValidDeviceID mencegah id perangkat dipakai untuk menyusupkan karakter ke nama topik MQTT.
func ValidDeviceID(id string) bool { return deviceID.MatchString(id) }

type Command struct {
	Action  string `json:"action"`
	URL     string `json:"url,omitempty"`
	StoryID *uint  `json:"story_id,omitempty"`
	Volume  *int   `json:"volume,omitempty"`
	Value   *int   `json:"value,omitempty"`
}

// Status adalah laporan terakhir yang diketahui dari sebuah totem.
type Status struct {
	Online    bool       `json:"online"`
	State     string     `json:"state"`
	Reason    *string    `json:"reason"`
	StoryID   *uint      `json:"story_id"`
	Position  int        `json:"position"`
	Duration  int        `json:"duration"`
	Volume    int        `json:"volume"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type Hub interface {
	Send(deviceID string, cmd Command) error
	Status(deviceID string) Status
	// Known: apakah totem ini pernah terlihat (mengirim online/status) sejak broker dipantau.
	Known(deviceID string) bool
}

// Disabled dipakai bila broker belum diatur; semua perintah ditolak dengan ErrNotConfigured.
type Disabled struct{}

func (Disabled) Send(string, Command) error { return ErrNotConfigured }
func (Disabled) Status(string) Status       { return Status{State: "unknown"} }
func (Disabled) Known(string) bool          { return false }
