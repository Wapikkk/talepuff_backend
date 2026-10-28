package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"talepuff_backend/models"
	"talepuff_backend/totem"
)

type sentCommand struct {
	DeviceID string
	Command  totem.Command
}

// fakeHub mengganti broker MQTT: mencatat perintah dan mengembalikan status yang diatur tes.
type fakeHub struct {
	mu       sync.Mutex
	sent     []sentCommand
	statuses map[string]totem.Status
	sendErr  error
}

func newFakeHub() *fakeHub { return &fakeHub{statuses: map[string]totem.Status{}} }

func (h *fakeHub) online(id string) {
	h.statuses[id] = totem.Status{Online: true, State: "idle"}
}

func (h *fakeHub) Send(id string, cmd totem.Command) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sendErr != nil {
		return h.sendErr
	}
	h.sent = append(h.sent, sentCommand{id, cmd})
	return nil
}

func (h *fakeHub) Status(id string) totem.Status {
	if s, ok := h.statuses[id]; ok {
		return s
	}
	return totem.Status{State: "unknown"}
}

func (h *fakeHub) Known(id string) bool { _, ok := h.statuses[id]; return ok }

func (h *fakeHub) commands() []sentCommand {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]sentCommand(nil), h.sent...)
}

func (e *env) pair(t *testing.T, uid, deviceID string) int {
	t.Helper()
	return e.call(t, uid, http.MethodPost, "/api/totem/pair", map[string]any{"device_id": deviceID}).Code
}

// errorCode lee `code` dari jawaban galat.
func errorCode(rec *httptest.ResponseRecorder) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Code
}

func TestTotem_ErrorsCarryAStableCode(t *testing.T) {
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	pair := func(uid, id string) *httptest.ResponseRecorder {
		return e.call(t, uid, http.MethodPost, "/api/totem/pair", map[string]any{"device_id": id})
	}

	mustEqual(t, "id buruk", errorCode(pair("uid-1", "x")), "invalid_device_id")
	mustEqual(t, "tidak terlihat", errorCode(pair("uid-1", "totem-001")), "totem_not_visible")

	command := map[string]any{"action": "pause"}
	mustEqual(t, "belum ditautkan", errorCode(e.call(t, "uid-1", http.MethodPost, "/api/totem/command", command)), "totem_not_paired")

	hub.online("totem-001")
	pair("uid-1", "totem-001")
	mustEqual(t, "milik akun lain", errorCode(pair("uid-2", "totem-001")), "totem_taken")

	hub.statuses["totem-001"] = totem.Status{Online: false}
	mustEqual(t, "mati", errorCode(e.call(t, "uid-1", http.MethodPost, "/api/totem/command", command)), "totem_offline")

	hub.online("totem-001")
	hub.sendErr = totem.ErrNotConnected
	mustEqual(t, "broker putus", errorCode(e.call(t, "uid-1", http.MethodPost, "/api/totem/command", command)), "totem_unreachable")
}

func TestTotem_EveryRouteRequiresAToken(t *testing.T) {
	e := newTestEnv(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/totem"},
		{http.MethodPost, "/api/totem/pair"},
		{http.MethodDelete, "/api/totem"},
		{http.MethodPost, "/api/totem/play"},
		{http.MethodPost, "/api/totem/command"},
	} {
		mustEqual(t, route.method+" "+route.path, e.call(t, "", route.method, route.path, nil).Code, http.StatusUnauthorized)
	}
}

func TestTotem_WithoutABrokerEveryCommandIs503(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	mustEqual(t, "pair", e.pair(t, "uid-1", "totem-001"), http.StatusServiceUnavailable)
	mustEqual(t, "command", e.call(t, "uid-1", http.MethodPost, "/api/totem/command", map[string]any{"action": "stop"}).Code, http.StatusServiceUnavailable)
	rec := e.call(t, "uid-1", http.MethodGet, "/api/totem", nil)
	mustEqual(t, "status tetap bisa dibaca", rec.Code, http.StatusOK)
}

func TestTotem_Pairing(t *testing.T) {
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")

	t.Run("id yang tidak valid ditolak", func(t *testing.T) {
		for _, bad := range []string{"", "ab", "Totem 001", "totem/001", "totem-001/#", "+", "-totem"} {
			mustEqual(t, bad, e.pair(t, "uid-1", bad), http.StatusBadRequest)
		}
	})

	t.Run("totem yang tidak terlihat di broker tidak bisa ditautkan", func(t *testing.T) {
		mustEqual(t, "status", e.pair(t, "uid-1", "totem-001"), http.StatusNotFound)
	})

	t.Run("menautkan, lalu membaca status", func(t *testing.T) {
		hub.online("totem-001")
		mustEqual(t, "pair", e.pair(t, "uid-1", "  TOTEM-001 "), http.StatusOK)
		mustEqual(t, "pair ulang idempoten", e.pair(t, "uid-1", "totem-001"), http.StatusOK)

		rec := e.call(t, "uid-1", http.MethodGet, "/api/totem", nil)
		var body struct {
			Paired   bool         `json:"paired"`
			DeviceID string       `json:"device_id"`
			Status   totem.Status `json:"status"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		mustEqual(t, "paired", body.Paired, true)
		mustEqual(t, "device_id", body.DeviceID, "totem-001")
		mustEqual(t, "online", body.Status.Online, true)
	})

	t.Run("totem milik akun lain tidak bisa direbut", func(t *testing.T) {
		mustEqual(t, "status", e.pair(t, "uid-2", "totem-001"), http.StatusConflict)
	})

	t.Run("akun lain tidak melihat totem itu", func(t *testing.T) {
		rec := e.call(t, "uid-2", http.MethodGet, "/api/totem", nil)
		mustEqual(t, "status", rec.Code, http.StatusOK)
		mustEqual(t, "isi", rec.Body.String(), `{"paired":false}`)
	})

	t.Run("totem baru menggantikan yang lama", func(t *testing.T) {
		hub.online("totem-002")
		mustEqual(t, "pair", e.pair(t, "uid-1", "totem-002"), http.StatusOK)
		var count int64
		e.db.Model(&models.Device{}).Count(&count)
		mustEqual(t, "jumlah tautan", count, int64(1))
		mustEqual(t, "totem lama bebas", e.pair(t, "uid-2", "totem-001"), http.StatusOK)
	})

	t.Run("melepas tautan menghentikan totem", func(t *testing.T) {
		before := len(hub.commands())
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, "/api/totem", nil).Code, http.StatusOK)
		cmds := hub.commands()
		mustEqual(t, "perintah stop terkirim", len(cmds), before+1)
		mustEqual(t, "aksi", cmds[len(cmds)-1].Command.Action, "stop")
		mustEqual(t, "ke totem lama", cmds[len(cmds)-1].DeviceID, "totem-002")
		mustEqual(t, "melepas lagi", e.call(t, "uid-1", http.MethodDelete, "/api/totem", nil).Code, http.StatusNotFound)
	})
}

func TestTotem_Play(t *testing.T) {
	t.Setenv("AUDIO_PUBLIC_BASE_URL", "http://172.20.10.8:8000/")
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "Moon", time.Hour, func(s *models.Story) {
		s.AudioURL = "http://localhost:8000/audio/story_1792526749_Brave.wav"
	})
	silent := e.seedStory(t, child.ID, "Silent", time.Hour, nil)
	play := func(uid string, body map[string]any) int {
		return e.call(t, uid, http.MethodPost, "/api/totem/play", body).Code
	}

	mustEqual(t, "belum menautkan totem", play("uid-1", map[string]any{"story_id": story.ID}), http.StatusNotFound)

	hub.online("totem-001")
	mustEqual(t, "pair", e.pair(t, "uid-1", "totem-001"), http.StatusOK)

	t.Run("mengirim alamat yang terjangkau totem, bukan localhost", func(t *testing.T) {
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": story.ID, "volume": 30}), http.StatusOK)
		cmds := hub.commands()
		mustEqual(t, "jumlah perintah", len(cmds), 1)
		mustEqual(t, "totem", cmds[0].DeviceID, "totem-001")
		mustEqual(t, "aksi", cmds[0].Command.Action, "play")
		mustEqual(t, "url", cmds[0].Command.URL, "http://172.20.10.8:8000/audio/story_1792526749_Brave.wav")
		mustEqual(t, "story_id", *cmds[0].Command.StoryID, story.ID)
		mustEqual(t, "volume", *cmds[0].Command.Volume, 30)

		var saved models.Story
		e.db.First(&saved, story.ID)
		mustEqual(t, "play_count naik", saved.PlayCount, 1)
	})

	t.Run("cerita orang lain tidak bisa diputar", func(t *testing.T) {
		e.register(t, "uid-3", "Cici")
		hub.online("totem-003")
		e.pair(t, "uid-3", "totem-003")
		mustEqual(t, "status", play("uid-3", map[string]any{"story_id": story.ID}), http.StatusNotFound)
	})

	t.Run("cerita tanpa suara", func(t *testing.T) {
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": silent.ID}), http.StatusConflict)
	})

	t.Run("permintaan tidak valid", func(t *testing.T) {
		mustEqual(t, "tanpa story_id", play("uid-1", map[string]any{}), http.StatusBadRequest)
		mustEqual(t, "volume 101", play("uid-1", map[string]any{"story_id": story.ID, "volume": 101}), http.StatusBadRequest)
		mustEqual(t, "volume negatif", play("uid-1", map[string]any{"story_id": story.ID, "volume": -1}), http.StatusBadRequest)
	})

	t.Run("totem mati", func(t *testing.T) {
		hub.statuses["totem-001"] = totem.Status{Online: false, State: "offline"}
		before := len(hub.commands())
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": story.ID}), http.StatusConflict)
		mustEqual(t, "tidak ada perintah terkirim", len(hub.commands()), before)
		hub.online("totem-001")
	})

	t.Run("broker bermasalah", func(t *testing.T) {
		hub.sendErr = totem.ErrNotConnected
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": story.ID}), http.StatusServiceUnavailable)
		hub.sendErr = nil
	})

	t.Run("alamat audio publik belum diatur", func(t *testing.T) {
		t.Setenv("AUDIO_PUBLIC_BASE_URL", "")
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": story.ID}), http.StatusServiceUnavailable)
	})

	t.Run("nama berkas yang tidak aman tidak dikirim", func(t *testing.T) {
		unsafe := e.seedStory(t, child.ID, "Unsafe", time.Hour, func(s *models.Story) {
			s.AudioURL = "http://localhost:8000/audio/..%2Fsecret.wav"
		})
		before := len(hub.commands())
		mustEqual(t, "status", play("uid-1", map[string]any{"story_id": unsafe.ID}), http.StatusServiceUnavailable)
		mustEqual(t, "tidak ada perintah terkirim", len(hub.commands()), before)
	})
}

func TestTotem_Command(t *testing.T) {
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	hub.online("totem-001")
	e.pair(t, "uid-1", "totem-001")
	send := func(body map[string]any) int {
		return e.call(t, "uid-1", http.MethodPost, "/api/totem/command", body).Code
	}

	for _, action := range []string{"pause", "resume", "stop"} {
		mustEqual(t, action, send(map[string]any{"action": action}), http.StatusOK)
	}
	mustEqual(t, "volume", send(map[string]any{"action": "volume", "value": 20}), http.StatusOK)

	cmds := hub.commands()
	mustEqual(t, "jumlah", len(cmds), 4)
	mustEqual(t, "aksi terakhir", cmds[3].Command.Action, "volume")
	mustEqual(t, "nilai", *cmds[3].Command.Value, 20)

	mustEqual(t, "aksi asing", send(map[string]any{"action": "reboot"}), http.StatusBadRequest)
	mustEqual(t, "volume tanpa nilai", send(map[string]any{"action": "volume"}), http.StatusBadRequest)
	mustEqual(t, "volume di luar batas", send(map[string]any{"action": "volume", "value": 101}), http.StatusBadRequest)
	mustEqual(t, "tidak ada perintah tambahan", len(hub.commands()), 4)

	hub.statuses["totem-001"] = totem.Status{Online: false}
	mustEqual(t, "totem mati", send(map[string]any{"action": "pause"}), http.StatusConflict)
}
