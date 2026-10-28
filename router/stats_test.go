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

type weeklyJSON struct {
	Days []struct {
		Date    string `json:"date"`
		Stories int    `json:"stories"`
		Plays   int    `json:"plays"`
		Seconds int    `json:"seconds"`
	} `json:"days"`
	Totals struct {
		Stories int `json:"stories"`
		Plays   int `json:"plays"`
		Seconds int `json:"seconds"`
	} `json:"totals"`
}

func (e *env) weekly(t *testing.T, uid string, childID uint) (weeklyJSON, int) {
	t.Helper()
	rec := e.call(t, uid, http.MethodGet, "/api/stats/weekly?child_id="+itoa(childID), nil)
	var body weeklyJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body, rec.Code
}

func (e *env) seedPlay(t *testing.T, story models.Story, device string, startedAt time.Time, seconds int) {
	t.Helper()
	play := models.StoryPlay{StoryID: story.ID, ChildID: story.ChildID, DeviceID: device, StartedAt: startedAt, ListenedSeconds: seconds}
	if err := e.db.Create(&play).Error; err != nil {
		t.Fatal(err)
	}
}

func withNow(t *testing.T, now time.Time) {
	t.Helper()
	handlers.Now = func() time.Time { return now }
	t.Cleanup(func() { handlers.Now = time.Now })
}

func TestWeeklyStats(t *testing.T) {
	withNow(t, at(28, 15, 0, 0))
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	other, _ := e.child(t, "uid-2", "uid-2")

	ago := func(days int, hour int) time.Duration { return at(28, 15, 0, 0).Sub(at(28-days, hour, 0, 0)) }
	s1 := e.seedStory(t, child.ID, "Today", ago(0, 9), nil)
	s2 := e.seedStory(t, child.ID, "Three days ago", ago(3, 20), nil)
	e.seedStory(t, child.ID, "Too old", ago(8, 12), nil)
	theirs := e.seedStory(t, other.ID, "Not mine", ago(0, 10), nil)

	e.seedPlay(t, s1, "totem-001", at(28, 10, 0, 0), 120)
	e.seedPlay(t, s1, "totem-001", at(28, 12, 0, 0), 60)
	e.seedPlay(t, s2, "totem-001", at(25, 21, 0, 0), 200)
	e.seedPlay(t, s2, "totem-001", at(20, 21, 0, 0), 999) // sebelum 7 hari terakhir
	e.seedPlay(t, theirs, "totem-009", at(28, 11, 0, 0), 500)

	t.Run("tujuh hari, yang terlama dulu, hari ini terakhir", func(t *testing.T) {
		body, code := e.weekly(t, "uid-1", child.ID)
		mustEqual(t, "status", code, http.StatusOK)
		mustEqual(t, "jumlah hari", len(body.Days), 7)
		mustEqual(t, "hari pertama", body.Days[0].Date, "2026-10-22")
		mustEqual(t, "hari terakhir", body.Days[6].Date, "2026-10-28")
	})

	t.Run("cerita, putaran, dan detik dihitung per hari", func(t *testing.T) {
		body, _ := e.weekly(t, "uid-1", child.ID)
		today, threeAgo := body.Days[6], body.Days[3]
		mustEqual(t, "cerita hari ini", today.Stories, 1)
		mustEqual(t, "putaran hari ini", today.Plays, 2)
		mustEqual(t, "detik hari ini", today.Seconds, 180)
		mustEqual(t, "cerita 3 hari lalu", threeAgo.Stories, 1)
		mustEqual(t, "putaran 3 hari lalu", threeAgo.Plays, 1)
		mustEqual(t, "detik 3 hari lalu", threeAgo.Seconds, 200)
		mustEqual(t, "hari kosong", body.Days[0].Plays+body.Days[1].Plays+body.Days[2].Plays, 0)
	})

	t.Run("total hanya memuat tujuh hari terakhir dan anak ini", func(t *testing.T) {
		body, _ := e.weekly(t, "uid-1", child.ID)
		mustEqual(t, "cerita", body.Totals.Stories, 2)
		mustEqual(t, "putaran", body.Totals.Plays, 3)
		mustEqual(t, "detik", body.Totals.Seconds, 380)
	})

	t.Run("anak lain hanya melihat statistiknya sendiri", func(t *testing.T) {
		body, code := e.weekly(t, "uid-2", other.ID)
		mustEqual(t, "status", code, http.StatusOK)
		mustEqual(t, "total putaran", body.Totals.Plays, 1)
		mustEqual(t, "detik", body.Totals.Seconds, 500)
	})

	t.Run("anak orang lain, id buruk, dan tanpa token", func(t *testing.T) {
		_, code := e.weekly(t, "uid-2", child.ID)
		mustEqual(t, "anak orang lain", code, http.StatusNotFound)
		mustEqual(t, "tanpa id", e.call(t, "uid-1", http.MethodGet, "/api/stats/weekly", nil).Code, http.StatusBadRequest)
		mustEqual(t, "id bukan angka", e.call(t, "uid-1", http.MethodGet, "/api/stats/weekly?child_id=x", nil).Code, http.StatusBadRequest)
		mustEqual(t, "tanpa token", e.call(t, "", http.MethodGet, "/api/stats/weekly?child_id=1", nil).Code, http.StatusUnauthorized)
	})
}

func TestStats_APlayIsLoggedWhenAStoryIsSentToTheTotem(t *testing.T) {
	t.Setenv("AUDIO_PUBLIC_BASE_URL", "http://172.20.10.8:8000")
	hub := newFakeHub()
	e := newTestEnvWithHub(t, hub)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "Moon", time.Hour, func(s *models.Story) { s.AudioURL = audioA })
	hub.online("totem-001")
	e.pair(t, "uid-1", "totem-001")

	rec := e.call(t, "uid-1", http.MethodPost, "/api/totem/play", map[string]any{"story_id": story.ID})
	mustEqual(t, "status", rec.Code, http.StatusOK)

	var plays []models.StoryPlay
	e.db.Find(&plays)
	mustEqual(t, "satu catatan", len(plays), 1)
	mustEqual(t, "cerita", plays[0].StoryID, story.ID)
	mustEqual(t, "anak", plays[0].ChildID, child.ID)
	mustEqual(t, "totem", plays[0].DeviceID, "totem-001")
	mustEqual(t, "belum ada lama dengar", plays[0].ListenedSeconds, 0)
}

func TestStats_ListeningTimeComesFromTheTotemReports(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "Moon", time.Hour, nil)
	other := e.seedStory(t, child.ID, "Other", time.Hour, nil)
	e.seedPlay(t, story, "totem-001", time.Now().Add(-time.Minute), 0)

	observe := handlers.RecordListening(e.db)
	listened := func() int {
		var play models.StoryPlay
		e.db.Where("story_id = ?", story.ID).First(&play)
		return play.ListenedSeconds
	}
	id := story.ID
	report := func(state string, position, duration int) totem.Status {
		return totem.Status{Online: true, State: state, StoryID: &id, Position: position, Duration: duration}
	}
	settle := func() { time.Sleep(50 * time.Millisecond) }

	t.Run("laporan pertama langsung dicatat", func(t *testing.T) {
		observe("totem-001", report("playing", 3, 36))
		mustEqual(t, "detik", listened(), 3)
	})

	t.Run("laporan playing berikutnya dibatasi agar database tidak ditulis tiap detik", func(t *testing.T) {
		observe("totem-001", report("playing", 4, 36))
		mustEqual(t, "tidak berubah", listened(), 3)
	})

	t.Run("jeda dicatat segera", func(t *testing.T) {
		observe("totem-001", report("paused", 12, 36))
		mustEqual(t, "detik", listened(), 12)
	})

	t.Run("posisi tidak pernah mundur", func(t *testing.T) {
		observe("totem-001", report("paused", 5, 36))
		mustEqual(t, "tetap", listened(), 12)
	})

	t.Run("selesai dicatat dan dipotong ke durasi cerita", func(t *testing.T) {
		observe("totem-001", report("idle", 99, 36))
		settle()
		mustEqual(t, "detik", listened(), 36)
	})

	t.Run("status tanpa cerita, posisi 0, atau keadaan lain diabaikan", func(t *testing.T) {
		e.db.Model(&models.StoryPlay{}).Where("story_id = ?", story.ID).Update("listened_seconds", 0)
		observe("totem-001", totem.Status{State: "idle", Position: 10})
		observe("totem-001", report("paused", 0, 36))
		observe("totem-001", report("error", 20, 36))
		observe("totem-001", report("loading", 20, 36))
		mustEqual(t, "tidak ada yang dicatat", listened(), 0)
	})

	t.Run("laporan untuk cerita lain atau totem lain tidak menyentuh catatan ini", func(t *testing.T) {
		otherID := other.ID
		observe("totem-001", totem.Status{State: "paused", StoryID: &otherID, Position: 30, Duration: 60})
		observe("totem-002", report("paused", 30, 60))
		mustEqual(t, "tetap 0", listened(), 0)
	})

	t.Run("pemutaran yang sudah lama tidak diperbarui lagi", func(t *testing.T) {
		e.db.Model(&models.StoryPlay{}).Where("story_id = ?", story.ID).Update("started_at", time.Now().Add(-48*time.Hour))
		observe("totem-001", report("paused", 30, 60))
		mustEqual(t, "tetap 0", listened(), 0)
	})
}
