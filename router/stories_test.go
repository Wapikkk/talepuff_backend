package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"talepuff_backend/handlers"
	"talepuff_backend/models"
)

type storyJSON struct {
	ID            uint   `json:"id"`
	ChildID       uint   `json:"child_id"`
	Title         string `json:"title"`
	Theme         string `json:"theme"`
	Mood          string `json:"mood"`
	LengthMinutes int    `json:"length_minutes"`
	Content       string `json:"content"`
	Description   string `json:"description"`
	AudioURL      string `json:"audio_url"`
	Sentences     []struct {
		Start int    `json:"start"`
		End   int    `json:"end"`
		Text  string `json:"text"`
	} `json:"sentences"`
	VoiceTone  string    `json:"voice_tone"`
	IsFavorite bool      `json:"is_favorite"`
	PlayCount  int       `json:"play_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type storyList struct {
	Stories []storyJSON `json:"stories"`
	HasMore bool        `json:"has_more"`
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) storyList {
	t.Helper()
	var list storyList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("balasan bukan daftar cerita: %v (%s)", err, rec.Body.String())
	}
	return list
}

func titles(list storyList) []string {
	names := make([]string, 0, len(list.Stories))
	for _, story := range list.Stories {
		names = append(names, story.Title)
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// seedStory menyimpan cerita langsung ke database; [age] mengatur "umur" cerita agar urutan terbaru terkendali.
func (e *env) seedStory(t *testing.T, childID uint, title string, age time.Duration, mutate func(*models.Story)) models.Story {
	t.Helper()
	story := models.Story{
		ChildID: childID, Title: title, Theme: "Daily", Mood: "Calm", LengthMinutes: 3,
		Content: "Once upon a time, " + title + ".", Description: "a description",
	}
	story.CreatedAt = time.Now().Add(-age)
	if mutate != nil {
		mutate(&story)
	}
	if err := e.db.Create(&story).Error; err != nil {
		t.Fatal(err)
	}
	return story
}

func (e *env) listStories(t *testing.T, uid string, query string) *httptest.ResponseRecorder {
	t.Helper()
	return e.call(t, uid, http.MethodGet, "/api/stories?"+query, nil)
}

func childQuery(child childJSON, extra string) string {
	return "child_id=" + itoa(child.ID) + extra
}

func TestStories_EveryRouteRequiresAToken(t *testing.T) {
	e := newTestEnv(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/stories?child_id=1"},
		{http.MethodGet, "/api/stories/1"},
		{http.MethodPut, "/api/stories/1/favorite"},
		{http.MethodDelete, "/api/stories/1"},
		{http.MethodPost, "/api/story/generate"},
		{http.MethodGet, "/api/story/jobs/1"},
	} {
		mustEqual(t, route.method+" "+route.path, e.call(t, "", route.method, route.path, nil).Code, http.StatusUnauthorized)
	}
}

func TestListStories_RecentIsNewestFirstAndOmitsTheStoryText(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	e.seedStory(t, child.ID, "Oldest", 3*time.Hour, nil)
	e.seedStory(t, child.ID, "Newest", 1*time.Minute, nil)
	e.seedStory(t, child.ID, "Middle", 1*time.Hour, nil)

	rec := e.listStories(t, "uid-1", childQuery(child, ""))

	mustEqual(t, "status", rec.Code, http.StatusOK)
	list := decodeList(t, rec)
	if !equalStrings(titles(list), []string{"Newest", "Middle", "Oldest"}) {
		t.Errorf("urutan %v, seharusnya terbaru dulu", titles(list))
	}
	mustEqual(t, "tanpa halaman berikutnya", list.HasMore, false)
	mustEqual(t, "isi cerita tidak ikut di daftar", list.Stories[0].Content, "")
	mustEqual(t, "deskripsi tidak ikut di daftar", list.Stories[0].Description, "")
	mustEqual(t, "mood ikut", list.Stories[0].Mood, "Calm")
	mustEqual(t, "id memakai kunci huruf kecil", list.Stories[0].ID != 0, true)
}

func TestListStories_EmptyIsAnEmptyArrayNotNull(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	rec := e.listStories(t, "uid-1", childQuery(child, ""))

	mustEqual(t, "status", rec.Code, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"stories":[]`) {
		t.Errorf("seharusnya array kosong, dapat %s", rec.Body.String())
	}
}

func TestListStories_Tabs(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	e.seedStory(t, child.ID, "Plain", 4*time.Hour, nil)
	e.seedStory(t, child.ID, "Loved old", 3*time.Hour, func(s *models.Story) { s.IsFavorite = true })
	e.seedStory(t, child.ID, "Loved new", 1*time.Hour, func(s *models.Story) { s.IsFavorite = true; s.PlayCount = 2 })
	e.seedStory(t, child.ID, "Hit", 2*time.Hour, func(s *models.Story) { s.PlayCount = 9 })

	favorites := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&tab=favorites")))
	if !equalStrings(titles(favorites), []string{"Loved new", "Loved old"}) {
		t.Errorf("favorit %v", titles(favorites))
	}

	popular := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&tab=popular")))
	if !equalStrings(titles(popular), []string{"Hit", "Loved new"}) {
		t.Errorf("populer %v, seharusnya yang paling sering diputar dulu dan tanpa yang belum pernah diputar", titles(popular))
	}
}

func TestListStories_Search(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	for i, title := range []string{"Aira's Space Adventure", "The Sleeping Cloud", "100% Fun", "snake_case story", "Plain story"} {
		e.seedStory(t, child.ID, title, time.Duration(i+1)*time.Hour, nil)
	}

	t.Run("tidak membedakan huruf besar-kecil dan mencari bagian judul", func(t *testing.T) {
		list := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&q=SPACE")))
		if !equalStrings(titles(list), []string{"Aira's Space Adventure"}) {
			t.Errorf("hasil %v", titles(list))
		}
	})

	t.Run("karakter khusus LIKE dicari apa adanya, bukan sebagai pola", func(t *testing.T) {
		percent := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&q=100%25")))
		if !equalStrings(titles(percent), []string{"100% Fun"}) {
			t.Errorf("'100%%' %v: tanda persen tidak boleh menjadi pola", titles(percent))
		}
		underscore := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&q=_")))
		if !equalStrings(titles(underscore), []string{"snake_case story"}) {
			t.Errorf("'_' %v: garis bawah tidak boleh cocok dengan sembarang karakter", titles(underscore))
		}
		onlyPercent := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&q=%25")))
		if !equalStrings(titles(onlyPercent), []string{"100% Fun"}) {
			t.Errorf("'%%' %v", titles(onlyPercent))
		}
	})

	t.Run("tanpa hasil memberi daftar kosong", func(t *testing.T) {
		mustEqual(t, "jumlah", len(decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&q=zzz"))).Stories), 0)
	})

	t.Run("pencarian digabung dengan tab", func(t *testing.T) {
		mustEqual(t, "favorit + cari", len(decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&tab=favorites&q=space"))).Stories), 0)
	})
}

func TestListStories_Pagination(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	for i := 1; i <= 5; i++ {
		e.seedStory(t, child.ID, "Story "+itoa(uint(i)), time.Duration(i)*time.Hour, nil)
	}

	first := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&limit=2")))
	if !equalStrings(titles(first), []string{"Story 1", "Story 2"}) || !first.HasMore {
		t.Errorf("halaman 1: %v hasMore=%v", titles(first), first.HasMore)
	}

	last := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&limit=2&offset=4")))
	if !equalStrings(titles(last), []string{"Story 5"}) || last.HasMore {
		t.Errorf("halaman terakhir: %v hasMore=%v", titles(last), last.HasMore)
	}

	exact := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "&limit=5")))
	mustEqual(t, "pas sebanyak batas: tidak ada halaman berikutnya", exact.HasMore, false)
}

func TestListStories_OnlyTheOwnersChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	aira, _ := e.child(t, "uid-1", "uid-1")
	bimo, _ := e.child(t, "uid-2", "uid-2")
	e.seedStory(t, aira.ID, "Aira private", time.Hour, nil)
	e.seedStory(t, bimo.ID, "Bimo story", time.Hour, nil)

	mustEqual(t, "anak orang lain", e.listStories(t, "uid-2", childQuery(aira, "")).Code, http.StatusNotFound)

	own := decodeList(t, e.listStories(t, "uid-2", childQuery(bimo, "")))
	if !equalStrings(titles(own), []string{"Bimo story"}) {
		t.Errorf("hanya cerita milik sendiri yang boleh tampil, dapat %v", titles(own))
	}
}

func TestListStories_RejectsBadParameters(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	for _, query := range []string{
		"",
		"child_id=abc",
		childQuery(child, "&tab=trending"),
		childQuery(child, "&limit=0"),
		childQuery(child, "&limit=101"),
		childQuery(child, "&limit=abc"),
		childQuery(child, "&offset=-1"),
		childQuery(child, "&q="+strings.Repeat("a", 101)),
	} {
		mustEqual(t, "status untuk '"+query+"'", e.listStories(t, "uid-1", query).Code, http.StatusBadRequest)
	}
}

func TestGetStory(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "My story", time.Hour, nil)
	path := "/api/stories/" + itoa(story.ID)

	t.Run("pemilik mendapat cerita lengkap", func(t *testing.T) {
		rec := e.call(t, "uid-1", http.MethodGet, path, nil)
		mustEqual(t, "status", rec.Code, http.StatusOK)
		var body struct {
			Data storyJSON `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		mustEqual(t, "id", body.Data.ID, story.ID)
		mustEqual(t, "isi cerita", body.Data.Content, "Once upon a time, My story.")
		mustEqual(t, "deskripsi", body.Data.Description, "a description")
	})

	t.Run("pengguna lain mendapat 404", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodGet, path, nil).Code, http.StatusNotFound)
	})

	t.Run("id bukan angka 400 dan yang tidak ada 404", func(t *testing.T) {
		mustEqual(t, "abc", e.call(t, "uid-1", http.MethodGet, "/api/stories/abc", nil).Code, http.StatusBadRequest)
		mustEqual(t, "9999", e.call(t, "uid-1", http.MethodGet, "/api/stories/9999", nil).Code, http.StatusNotFound)
	})
}

func TestSetStoryFavorite(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "My story", time.Hour, nil)
	path := "/api/stories/" + itoa(story.ID) + "/favorite"

	stored := func() bool {
		var saved models.Story
		e.db.First(&saved, story.ID)
		return saved.IsFavorite
	}

	t.Run("menandai lalu melepas favorit", func(t *testing.T) {
		mustEqual(t, "tandai", e.call(t, "uid-1", http.MethodPut, path, map[string]bool{"favorite": true}).Code, http.StatusOK)
		mustEqual(t, "tersimpan", stored(), true)
		mustEqual(t, "lepas", e.call(t, "uid-1", http.MethodPut, path, map[string]bool{"favorite": false}).Code, http.StatusOK)
		mustEqual(t, "tersimpan", stored(), false)
	})

	t.Run("nilai false eksplisit diterima, tetapi field yang hilang ditolak", func(t *testing.T) {
		mustEqual(t, "kosong", e.call(t, "uid-1", http.MethodPut, path, map[string]string{}).Code, http.StatusBadRequest)
		mustEqual(t, "bukan boolean", e.call(t, "uid-1", http.MethodPut, path, map[string]string{"favorite": "yes"}).Code, http.StatusBadRequest)
	})

	t.Run("pengguna lain tidak bisa mengubah favorit orang lain", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodPut, path, map[string]bool{"favorite": true}).Code, http.StatusNotFound)
		mustEqual(t, "tidak berubah", stored(), false)
	})
}

func TestDeleteStory(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")

	var removed []string
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			removed = append(removed, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	exists := func(id uint) bool {
		var count int64
		e.db.Model(&models.Story{}).Where("id = ?", id).Count(&count)
		return count == 1
	}

	t.Run("pengguna lain tidak bisa menghapus, ceritanya tetap ada", func(t *testing.T) {
		story := e.seedStory(t, child.ID, "Not yours", time.Hour, nil)
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodDelete, "/api/stories/"+itoa(story.ID), nil).Code, http.StatusNotFound)
		mustEqual(t, "masih ada", exists(story.ID), true)
	})

	t.Run("pemilik menghapus cerita, dan file audionya diminta dihapus", func(t *testing.T) {
		removed = nil
		story := e.seedStory(t, child.ID, "With audio", time.Hour, func(s *models.Story) {
			s.AudioURL = "http://localhost:8000/audio/story_123_Warm.wav"
		})
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, "/api/stories/"+itoa(story.ID), nil).Code, http.StatusOK)
		mustEqual(t, "sudah hilang", exists(story.ID), false)
		mustEqual(t, "audio diminta dihapus", len(removed), 1)
		mustEqual(t, "jalur", removed[0], "/generated_audio/story_123_Warm.wav")
		for _, title := range titles(decodeList(t, e.listStories(t, "uid-1", childQuery(child, "")))) {
			if title == "With audio" {
				t.Errorf("cerita yang dihapus masih muncul di daftar")
			}
		}
	})

	t.Run("cerita tanpa audio dihapus tanpa memanggil layanan AI", func(t *testing.T) {
		removed = nil
		story := e.seedStory(t, child.ID, "No audio", time.Hour, nil)
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, "/api/stories/"+itoa(story.ID), nil).Code, http.StatusOK)
		mustEqual(t, "tidak memanggil AI", len(removed), 0)
	})

	t.Run("cerita tetap terhapus walau layanan AI mati", func(t *testing.T) {
		story := e.seedStory(t, child.ID, "AI down", time.Hour, func(s *models.Story) {
			s.AudioURL = "http://localhost:8000/audio/story_9_Brave.wav"
		})
		t.Setenv("AI_SERVICE_URL", "http://127.0.0.1:1")
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, "/api/stories/"+itoa(story.ID), nil).Code, http.StatusOK)
		mustEqual(t, "sudah hilang", exists(story.ID), false)
	})

	t.Run("id tidak valid dan cerita yang sudah terhapus", func(t *testing.T) {
		mustEqual(t, "bukan angka", e.call(t, "uid-1", http.MethodDelete, "/api/stories/abc", nil).Code, http.StatusBadRequest)
		mustEqual(t, "tidak ada", e.call(t, "uid-1", http.MethodDelete, "/api/stories/999999", nil).Code, http.StatusNotFound)
	})
}

// ---------- pembuatan cerita dan audio dengan layanan AI palsu ----------

func fakeAI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("AI_SERVICE_URL", server.URL)
	return server
}

type jobJSON struct {
	ID           uint       `json:"id"`
	Status       string     `json:"status"`
	Story        *storyJSON `json:"story"`
	ErrorCode    string     `json:"error_code"`
	IdeaDropped  bool       `json:"idea_dropped"`
	NameReplaced bool       `json:"name_replaced"`
}

func jobPayload(child childJSON, overrides map[string]any) map[string]any {
	payload := map[string]any{
		"child_id": child.ID, "character_name": "Aira", "theme": "Daily", "mood": "Calm",
		"length_minutes": 3, "language": "Basic English", "voice_tone": "Warm",
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return payload
}

func (e *env) startJob(t *testing.T, uid string, payload map[string]any) (uint, *httptest.ResponseRecorder) {
	t.Helper()
	rec := e.call(t, uid, http.MethodPost, "/api/story/generate", payload)
	var body struct {
		JobID uint `json:"job_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.JobID, rec
}

func (e *env) job(t *testing.T, uid string, id uint) (jobJSON, int) {
	t.Helper()
	rec := e.call(t, uid, http.MethodGet, "/api/story/jobs/"+itoa(id), nil)
	var job jobJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	return job, rec.Code
}

func (e *env) waitForStatus(t *testing.T, uid string, id uint, want ...string) jobJSON {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, code := e.job(t, uid, id)
		mustEqual(t, "status HTTP", code, http.StatusOK)
		for _, status := range want {
			if job.Status == status {
				return job
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d berstatus %q, menunggu %v", id, job.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *env) finishJob(t *testing.T, uid string, id uint) jobJSON {
	t.Helper()
	return e.waitForStatus(t, uid, id, "done", "failed")
}

func releaseOnce(t *testing.T, gates ...chan struct{}) {
	t.Helper()
	t.Cleanup(func() {
		for _, gate := range gates {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
	})
}

func healthyAI(t *testing.T, received *map[string]any) {
	t.Helper()
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_story":
			if received != nil {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, received)
			}
			_, _ = w.Write([]byte(`{"status":"success","title":"Moon Rocket","response":"Aira flies to the moon.","flags":[]}`))
		case "/generate_audio":
			_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav","sentences":[{"start":0,"end":1500,"text":"Aira flies"},{"start":1450,"end":2600,"text":"to the moon."}]}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestStoryJob_WritesTheStoryThenTheVoiceAndSavesEverythingTheListNeeds(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	var received map[string]any
	healthyAI(t, &received)

	id, rec := e.startJob(t, "uid-1", jobPayload(child, map[string]any{
		"theme": "Adventure", "mood": "Calm, Brave", "length_minutes": 5, "description": "A trip to the moon",
	}))
	mustEqual(t, "status", rec.Code, http.StatusAccepted)
	if id == 0 {
		t.Fatalf("job_id kosong: %s", rec.Body.String())
	}

	job := e.finishJob(t, "uid-1", id)
	mustEqual(t, "selesai", job.Status, "done")
	mustEqual(t, "mood diteruskan ke AI", received["mood"], any("Calm, Brave"))
	if job.Story == nil || job.Story.ID == 0 {
		t.Fatalf("job selesai harus membawa cerita lengkap: %+v", job)
	}
	mustEqual(t, "judul", job.Story.Title, "Moon Rocket")
	mustEqual(t, "isi", job.Story.Content, "Aira flies to the moon.")
	mustEqual(t, "audio", job.Story.AudioURL, "http://audio.example/a.wav")
	mustEqual(t, "suara", job.Story.VoiceTone, "Warm")

	var saved models.Story
	if err := e.db.First(&saved, job.Story.ID).Error; err != nil {
		t.Fatalf("cerita tidak tersimpan: %v", err)
	}
	mustEqual(t, "mood tersimpan", saved.Mood, "Calm, Brave")
	mustEqual(t, "deskripsi tersimpan", saved.Description, "A trip to the moon")
	mustEqual(t, "durasi tersimpan", saved.LengthMinutes, 5)
	mustEqual(t, "tema tersimpan", saved.Theme, "Adventure")
	mustEqual(t, "audio tersimpan", saved.AudioURL, "http://audio.example/a.wav")

	mustEqual(t, "kalimat ikut di jawaban job", len(job.Story.Sentences), 2)
	mustEqual(t, "teks kalimat", job.Story.Sentences[1].Text, "to the moon.")
	mustEqual(t, "waktu mulai", job.Story.Sentences[1].Start, 1450)
	detail := e.call(t, "uid-1", http.MethodGet, "/api/stories/"+itoa(job.Story.ID), nil)
	var detailBody struct {
		Data storyJSON `json:"data"`
	}
	_ = json.Unmarshal(detail.Body.Bytes(), &detailBody)
	mustEqual(t, "kalimat ikut di detail", len(detailBody.Data.Sentences), 2)
	mustEqual(t, "daftar tidak membawa kalimat", len(decodeList(t, e.listStories(t, "uid-1", childQuery(child, ""))).Stories[0].Sentences), 0)

	mustEqual(t, "cerita baru ada di playlist", len(decodeList(t, e.listStories(t, "uid-1", childQuery(child, ""))).Stories), 1)
}

func TestStoryJob_ReportsTheRealStageWhileItWorks(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	releaseStory, releaseVoice := make(chan struct{}), make(chan struct{})
	releaseOnce(t, releaseStory, releaseVoice)
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/generate_story" {
			<-releaseStory
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Once.","flags":[]}`))
			return
		}
		<-releaseVoice
		_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
	})

	id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))

	e.waitForStatus(t, "uid-1", id, "writing_story")
	job, _ := e.job(t, "uid-1", id)
	if job.Story != nil {
		t.Errorf("belum ada cerita saat masih menulis")
	}
	close(releaseStory)

	job = e.waitForStatus(t, "uid-1", id, "creating_voice")
	if job.Story == nil || job.Story.Title != "T" {
		t.Errorf("saat membuat suara, ceritanya sudah tersimpan dan ikut dilaporkan: %+v", job)
	}
	close(releaseVoice)

	mustEqual(t, "akhirnya", e.finishJob(t, "uid-1", id).Status, "done")
}

func TestStoryJob_ValidatesTheRequestBeforeDoingAnyWork(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	calls := 0
	var received map[string]any
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/generate_story" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Once.","flags":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
	})

	for _, language := range []string{"Mixed (EN + ID)", "Bahasa Indonesia", "basic english"} {
		_, rec := e.startJob(t, "uid-1", jobPayload(child, map[string]any{"language": language}))
		mustEqual(t, "bahasa ditolak: "+language, rec.Code, http.StatusBadRequest)
	}
	for _, voice := range []any{"", "Robot", nil} {
		_, rec := e.startJob(t, "uid-1", jobPayload(child, map[string]any{"voice_tone": voice}))
		mustEqual(t, "suara ditolak", rec.Code, http.StatusBadRequest)
	}
	_, rec := e.startJob(t, "uid-2", jobPayload(child, nil))
	mustEqual(t, "anak milik orang lain", rec.Code, http.StatusNotFound)

	var jobs int64
	e.db.Model(&models.StoryJob{}).Count(&jobs)
	mustEqual(t, "tidak ada job yang dibuat", jobs, int64(0))
	mustEqual(t, "AI tidak dipanggil", calls, 0)

	for _, language := range []string{"Basic English", "", "  Basic English  "} {
		id, rec := e.startJob(t, "uid-1", jobPayload(child, map[string]any{"language": language}))
		mustEqual(t, "diterima: '"+language+"'", rec.Code, http.StatusAccepted)
		e.finishJob(t, "uid-1", id)
		mustEqual(t, "yang dikirim ke AI", received["language"], any("Basic English"))
	}
}

func TestStoryJob_FailuresSayWhatWentWrongAndSaveNothingWhenWritingFails(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	storyCount := func() int64 {
		var count int64
		e.db.Model(&models.Story{}).Count(&count)
		return count
	}

	cases := []struct {
		name string
		ai   func(t *testing.T)
		code string
	}{
		{"layanan AI mati", func(t *testing.T) { t.Setenv("AI_SERVICE_URL", "http://127.0.0.1:1") }, "ai_unavailable"},
		{"AI menjawab 502", func(t *testing.T) {
			fakeAI(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusBadGateway) })
		}, "ai_failed"},
		{"AI menjawab bukan JSON", func(t *testing.T) {
			fakeAI(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) })
		}, "ai_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.ai(t)
			id, rec := e.startJob(t, "uid-1", jobPayload(child, nil))
			mustEqual(t, "diterima", rec.Code, http.StatusAccepted)
			job := e.finishJob(t, "uid-1", id)
			mustEqual(t, "status", job.Status, "failed")
			mustEqual(t, "kode", job.ErrorCode, tc.code)
			mustEqual(t, "tidak ada cerita tersimpan", storyCount(), int64(0))
		})
	}

	t.Run("suara gagal: ceritanya tetap tersimpan", func(t *testing.T) {
		fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/generate_story" {
				_, _ = w.Write([]byte(`{"status":"success","title":"Kept","response":"Once.","flags":[]}`))
				return
			}
			http.Error(w, "tts down", http.StatusInternalServerError)
		})
		id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
		job := e.finishJob(t, "uid-1", id)
		mustEqual(t, "status", job.Status, "failed")
		mustEqual(t, "kode", job.ErrorCode, "voice_failed")
		if job.Story == nil || job.Story.Title != "Kept" {
			t.Fatalf("cerita yang sudah tertulis harus tetap dilaporkan: %+v", job)
		}
		mustEqual(t, "cerita ada", storyCount(), int64(1))
		mustEqual(t, "tanpa audio", job.Story.AudioURL, "")
	})
}

func TestStoryJob_TellsWhenTheIdeaWasNotUsed(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	flags := `["repaired:1"]`
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/generate_story" {
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Once.","flags":` + flags + `}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
	})

	id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
	mustEqual(t, "ide dipakai", e.finishJob(t, "uid-1", id).IdeaDropped, false)

	flags = `["idea_dropped","repaired:1"]`
	id, _ = e.startJob(t, "uid-1", jobPayload(child, nil))
	dropped := e.finishJob(t, "uid-1", id)
	mustEqual(t, "ide dibuang", dropped.IdeaDropped, true)
	mustEqual(t, "nama tetap dipakai", dropped.NameReplaced, false)

	flags = `["name_replaced"]`
	id, _ = e.startJob(t, "uid-1", jobPayload(child, nil))
	replaced := e.finishJob(t, "uid-1", id)
	mustEqual(t, "nama diganti", replaced.NameReplaced, true)
	mustEqual(t, "ide tetap dipakai", replaced.IdeaDropped, false)
}

func TestStoryJob_OnlyOneActiveJobPerChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	release := make(chan struct{})
	releaseOnce(t, release)
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
		if r.URL.Path == "/generate_story" {
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Once.","flags":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
	})

	first, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
	_, rec := e.startJob(t, "uid-1", jobPayload(child, nil))
	mustEqual(t, "job kedua ditolak", rec.Code, http.StatusConflict)
	var conflict struct {
		JobID uint `json:"job_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &conflict)
	mustEqual(t, "menunjuk job yang sedang berjalan", conflict.JobID, first)

	close(release)
	e.finishJob(t, "uid-1", first)

	second, rec := e.startJob(t, "uid-1", jobPayload(child, nil))
	mustEqual(t, "setelah selesai boleh lagi", rec.Code, http.StatusAccepted)
	e.finishJob(t, "uid-1", second)
}

func TestStoryJob_OnlyTheOwnerCanSeeIt(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	healthyAI(t, nil)

	id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
	e.finishJob(t, "uid-1", id)

	_, code := e.job(t, "uid-2", id)
	mustEqual(t, "pengguna lain", code, http.StatusNotFound)
	_, code = e.job(t, "uid-1", 999999)
	mustEqual(t, "tidak ada", code, http.StatusNotFound)
	mustEqual(t, "id bukan angka", e.call(t, "uid-1", http.MethodGet, "/api/story/jobs/abc", nil).Code, http.StatusBadRequest)
}

func TestStoryJob_ARestartClosesJobsThatWereInFlight(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	healthyAI(t, nil)

	stuck := models.StoryJob{ChildID: child.ID, Status: models.JobWritingStory}
	e.db.Create(&stuck)
	_, rec := e.startJob(t, "uid-1", jobPayload(child, nil))
	mustEqual(t, "job lama yang macet memblokir", rec.Code, http.StatusConflict)

	handlers.FailInterruptedJobs(e.db)

	job, _ := e.job(t, "uid-1", stuck.ID)
	mustEqual(t, "status", job.Status, "failed")
	mustEqual(t, "kode", job.ErrorCode, "interrupted")
	id, rec := e.startJob(t, "uid-1", jobPayload(child, nil))
	mustEqual(t, "sekarang bisa membuat lagi", rec.Code, http.StatusAccepted)
	e.finishJob(t, "uid-1", id)
}

func TestGenerateAudio_RecordsTheVoiceAndRejectsUnknownVoices(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	story := e.seedStory(t, child.ID, "My story", time.Hour, nil)

	calls := 0
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		mustEqual(t, "jalur", r.URL.Path, "/generate_audio")
		_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
	})
	path := "/api/audio/generate_audio"

	mustEqual(t, "suara tidak dikenal ditolak", e.call(t, "uid-1", http.MethodPost, path, map[string]any{"story_id": story.ID, "voice_tone": "Robot"}).Code, http.StatusBadRequest)
	mustEqual(t, "AI tidak dipanggil untuk suara yang salah", calls, 0)

	rec := e.call(t, "uid-1", http.MethodPost, path, map[string]any{"story_id": story.ID, "voice_tone": "Warm"})
	mustEqual(t, "status", rec.Code, http.StatusOK)

	var saved models.Story
	e.db.First(&saved, story.ID)
	mustEqual(t, "suara tersimpan", saved.VoiceTone, "Warm")
	mustEqual(t, "URL audio tersimpan", saved.AudioURL, "http://audio.example/a.wav")
}

func TestHealth_IsPublicSoTheAppCanTestAnAddressBeforeLoggingIn(t *testing.T) {
	e := newTestEnv(t)

	rec := e.call(t, "", http.MethodGet, "/api/health", nil)

	mustEqual(t, "status", rec.Code, http.StatusOK)
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	mustEqual(t, "service", body["service"], "talepuff")
	mustEqual(t, "status", body["status"], "ok")
}

func TestStoryJob_CleansUpSentenceTimingsFromTheAIService(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_story":
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Hello there.","flags":[]}`))
		case "/generate_audio":
			_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav","sentences":[
				{"start":-50,"end":-10,"text":"  Hello  "},
				{"start":900,"end":100,"text":"there."},
				{"start":2000,"end":3000,"text":"   "}]}`))
		default:
			http.NotFound(w, r)
		}
	})

	id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
	job := e.finishJob(t, "uid-1", id)

	mustEqual(t, "kalimat kosong dibuang", len(job.Story.Sentences), 2)
	mustEqual(t, "teks dirapikan", job.Story.Sentences[0].Text, "Hello")
	mustEqual(t, "mulai tidak negatif", job.Story.Sentences[0].Start, 0)
	mustEqual(t, "selesai tidak sebelum mulai", job.Story.Sentences[0].End, 0)
	mustEqual(t, "akhir tidak mundur dari awal", job.Story.Sentences[1].End, 900)
}

func TestStoryJob_AStoryWithoutTimingsStillWorks(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_story":
			_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Hello there.","flags":[]}`))
		case "/generate_audio":
			_, _ = w.Write([]byte(`{"status":"success","audio_url":"http://audio.example/a.wav"}`))
		default:
			http.NotFound(w, r)
		}
	})

	id, _ := e.startJob(t, "uid-1", jobPayload(child, nil))
	job := e.finishJob(t, "uid-1", id)

	mustEqual(t, "selesai", job.Status, "done")
	mustEqual(t, "tanpa kalimat", len(job.Story.Sentences), 0)
}
