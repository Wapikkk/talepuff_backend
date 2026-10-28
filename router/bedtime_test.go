package router

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"talepuff_backend/handlers"
	"talepuff_backend/models"
	"talepuff_backend/totem"
)

const audioA = "http://localhost:8000/audio/story_1792526749_Brave.wav"
const audioB = "http://localhost:8000/audio/story_1792526800_Warm.wav"

func (e *env) setBedtime(t *testing.T, uid string, enabled bool, hour, minute int) int {
	t.Helper()
	return e.call(t, uid, http.MethodPut, "/api/totem/bedtime", map[string]any{"enabled": enabled, "hour": hour, "minute": minute}).Code
}

func at(day, hour, minute, second int) time.Time {
	return time.Date(2026, 10, day, hour, minute, second, 0, time.Local)
}

func TestBedtime_ScheduleApi(t *testing.T) {
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	hub.online("totem-001")
	e.pair(t, "uid-1", "totem-001")

	t.Run("bawaan: mati, jam 20:00", func(t *testing.T) {
		rec := e.call(t, "uid-1", http.MethodGet, "/api/totem", nil)
		var body struct {
			Bedtime struct {
				Enabled bool `json:"enabled"`
				Hour    int  `json:"hour"`
				Minute  int  `json:"minute"`
			} `json:"bedtime"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		mustEqual(t, "aktif", body.Bedtime.Enabled, false)
		mustEqual(t, "jam", body.Bedtime.Hour, 20)
		mustEqual(t, "menit", body.Bedtime.Minute, 0)
	})

	t.Run("menyimpan jadwal", func(t *testing.T) {
		mustEqual(t, "status", e.setBedtime(t, "uid-1", true, 19, 45), http.StatusOK)
		var saved models.Device
		e.db.Where("device_id = ?", "totem-001").First(&saved)
		mustEqual(t, "aktif", saved.BedtimeEnabled, true)
		mustEqual(t, "jam", saved.BedtimeHour, 19)
		mustEqual(t, "menit", saved.BedtimeMinute, 45)
	})

	t.Run("nilai yang tidak valid ditolak", func(t *testing.T) {
		mustEqual(t, "jam 24", e.setBedtime(t, "uid-1", true, 24, 0), http.StatusBadRequest)
		mustEqual(t, "jam negatif", e.setBedtime(t, "uid-1", true, -1, 0), http.StatusBadRequest)
		mustEqual(t, "menit 60", e.setBedtime(t, "uid-1", true, 8, 60), http.StatusBadRequest)
		rec := e.call(t, "uid-1", http.MethodPut, "/api/totem/bedtime", map[string]any{"enabled": true})
		mustEqual(t, "tanpa jam", rec.Code, http.StatusBadRequest)
	})

	t.Run("tanpa totem tertaut", func(t *testing.T) {
		rec := e.call(t, "uid-2", http.MethodPut, "/api/totem/bedtime", map[string]any{"enabled": true, "hour": 8, "minute": 0})
		mustEqual(t, "status", rec.Code, http.StatusNotFound)
		mustEqual(t, "kode", errorCode(rec), "totem_not_paired")
	})
}

func TestBedtime_Scheduler(t *testing.T) {
	t.Setenv("AUDIO_PUBLIC_BASE_URL", "http://172.20.10.8:8000")
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	childOne, _ := e.child(t, "uid-1", "uid-1")
	childTwo, _ := e.child(t, "uid-2", "uid-2")
	hub.online("totem-001")
	e.pair(t, "uid-1", "totem-001")

	older := e.seedStory(t, childOne.ID, "Older", 3*time.Hour, func(s *models.Story) { s.AudioURL = audioA })
	newest := e.seedStory(t, childOne.ID, "Newest", time.Hour, func(s *models.Story) { s.AudioURL = audioB })
	e.seedStory(t, childOne.ID, "Newest but silent", time.Minute, nil)
	e.seedStory(t, childTwo.ID, "Someone else's", time.Second, func(s *models.Story) { s.AudioURL = audioA })
	_, _ = older, newest

	run := func(now time.Time) { handlers.RunBedtimeOnce(e.db, hub, now) }
	plays := func() []sentCommand {
		var out []sentCommand
		for _, c := range hub.commands() {
			if c.Command.Action == "play" {
				out = append(out, c)
			}
		}
		return out
	}

	mustEqual(t, "jadwal 20:00", e.setBedtime(t, "uid-1", true, 20, 0), http.StatusOK)

	t.Run("belum waktunya", func(t *testing.T) {
		run(at(28, 19, 59, 59))
		mustEqual(t, "tidak ada yang diputar", len(plays()), 0)
	})

	t.Run("saat waktunya memutar cerita bersuara terbaru milik akun itu", func(t *testing.T) {
		run(at(28, 20, 0, 5))
		got := plays()
		mustEqual(t, "satu perintah", len(got), 1)
		mustEqual(t, "totem", got[0].DeviceID, "totem-001")
		mustEqual(t, "alamat audio", got[0].Command.URL, "http://172.20.10.8:8000/audio/story_1792526800_Warm.wav")
		mustEqual(t, "id cerita", *got[0].Command.StoryID, newest.ID)

		var saved models.Story
		e.db.First(&saved, newest.ID)
		mustEqual(t, "play_count naik", saved.PlayCount, 1)
	})

	t.Run("tidak diputar dua kali pada hari yang sama", func(t *testing.T) {
		run(at(28, 20, 0, 25))
		run(at(28, 20, 5, 0))
		mustEqual(t, "tetap satu", len(plays()), 1)
	})

	t.Run("besok diputar lagi", func(t *testing.T) {
		run(at(29, 20, 0, 10))
		mustEqual(t, "dua", len(plays()), 2)
	})

	t.Run("mengubah jadwal membuka kembali hari ini", func(t *testing.T) {
		mustEqual(t, "jadwal baru 21:00", e.setBedtime(t, "uid-1", true, 21, 0), http.StatusOK)
		run(at(29, 21, 0, 1))
		mustEqual(t, "tiga", len(plays()), 3)
	})

	t.Run("jadwal dimatikan", func(t *testing.T) {
		e.setBedtime(t, "uid-1", false, 21, 0)
		run(at(30, 21, 0, 1))
		mustEqual(t, "tetap tiga", len(plays()), 3)
	})

	t.Run("volume dipotong ke batas orang tua", func(t *testing.T) {
		e.call(t, "uid-1", http.MethodPut, "/api/totem/volume-limit", map[string]any{"limit": 25})
		hub.statuses["totem-001"] = totem.Status{Online: true, State: "idle", Volume: 70}
		e.setBedtime(t, "uid-1", true, 22, 0)
		run(at(30, 22, 0, 1))
		got := plays()
		mustEqual(t, "empat", len(got), 4)
		mustEqual(t, "volume", *got[3].Command.Volume, 25)
	})
}

func TestBedtime_SchedulerDoesNotForceItself(t *testing.T) {
	t.Setenv("AUDIO_PUBLIC_BASE_URL", "http://172.20.10.8:8000")
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	e.seedStory(t, child.ID, "Story", time.Hour, func(s *models.Story) { s.AudioURL = audioA })
	hub.online("totem-001")
	e.pair(t, "uid-1", "totem-001")
	e.setBedtime(t, "uid-1", true, 20, 0)
	run := func(now time.Time) { handlers.RunBedtimeOnce(e.db, hub, now) }
	plays := func() int {
		n := 0
		for _, c := range hub.commands() {
			if c.Command.Action == "play" {
				n++
			}
		}
		return n
	}

	t.Run("totem mati: dicoba lagi sampai masih dalam jendela waktu", func(t *testing.T) {
		hub.statuses["totem-001"] = totem.Status{Online: false, State: "offline"}
		run(at(28, 20, 0, 5))
		run(at(28, 20, 4, 0))
		mustEqual(t, "belum ada", plays(), 0)

		hub.online("totem-001")
		run(at(28, 20, 6, 0))
		mustEqual(t, "diputar begitu totem hidup", plays(), 1)
	})

	t.Run("lewat jendela waktu hari itu dilewati", func(t *testing.T) {
		e.setBedtime(t, "uid-1", true, 20, 0)
		hub.statuses["totem-001"] = totem.Status{Online: false}
		run(at(29, 20, 0, 5))
		hub.online("totem-001")
		run(at(29, 20, 11, 0))
		mustEqual(t, "terlambat", plays(), 1)
	})

	t.Run("tidak memotong cerita yang sedang diputar", func(t *testing.T) {
		e.setBedtime(t, "uid-1", true, 20, 0)
		hub.statuses["totem-001"] = totem.Status{Online: true, State: "playing"}
		run(at(30, 20, 0, 5))
		hub.online("totem-001")
		run(at(30, 20, 1, 0))
		mustEqual(t, "tidak diputar setelahnya juga", plays(), 1)
	})

	t.Run("akun tanpa cerita bersuara tidak memutar apa pun", func(t *testing.T) {
		e.register(t, "uid-9", "Cici")
		hub.online("totem-009")
		e.pair(t, "uid-9", "totem-009")
		e.setBedtime(t, "uid-9", true, 20, 0)

		run(at(31, 20, 0, 5))

		for _, c := range hub.commands() {
			if c.DeviceID == "totem-009" {
				t.Fatalf("totem tanpa cerita bersuara menerima perintah: %+v", c)
			}
		}
	})
}
