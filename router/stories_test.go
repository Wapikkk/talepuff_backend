package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"talepuff_backend/models"
)

type storyJSON struct {
	ID            uint      `json:"id"`
	ChildID       uint      `json:"child_id"`
	Title         string    `json:"title"`
	Theme         string    `json:"theme"`
	Mood          string    `json:"mood"`
	LengthMinutes int       `json:"length_minutes"`
	Content       string    `json:"content"`
	Description   string    `json:"description"`
	AudioURL      string    `json:"audio_url"`
	VoiceTone     string    `json:"voice_tone"`
	IsFavorite    bool      `json:"is_favorite"`
	PlayCount     int       `json:"play_count"`
	CreatedAt     time.Time `json:"created_at"`
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
	} {
		mustEqual(t, route.method+" "+route.path, e.call(t, "", route.method, route.path, nil).Code, http.StatusUnauthorized)
	}
}

func TestListStories_RecentIsNewestFirstAndOmitsTheStoryText(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	rec := e.listStories(t, "uid-1", childQuery(child, ""))

	mustEqual(t, "status", rec.Code, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"stories":[]`) {
		t.Errorf("seharusnya array kosong, dapat %s", rec.Body.String())
	}
}

func TestListStories_Tabs(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
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
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
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

// ---------- pembuatan cerita dan audio dengan layanan AI palsu ----------

func fakeAI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("AI_SERVICE_URL", server.URL)
	return server
}

func TestGenerateStory_SavesEverythingTheListNeedsAndAnswersWithLowercaseKeys(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	var received map[string]any
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		mustEqual(t, "jalur", r.URL.Path, "/generate_story")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		_, _ = w.Write([]byte(`{"status":"success","title":"Moon Rocket","response":"Aira flies to the moon."}`))
	})

	rec := e.call(t, "uid-1", http.MethodPost, "/api/story/generate", map[string]any{
		"child_id": child.ID, "character_name": "Aira", "theme": "Adventure", "mood": "Calm, Brave",
		"length_minutes": 5, "language": "Basic English", "description": "A trip to the moon",
	})

	mustEqual(t, "status", rec.Code, http.StatusOK)
	mustEqual(t, "mood diteruskan ke AI", received["mood"], any("Calm, Brave"))

	var body struct {
		Data storyJSON `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID == 0 {
		t.Fatalf("data.id kosong: aplikasi membaca kunci 'id' (bukan 'ID'). Balasan: %s", rec.Body.String())
	}
	mustEqual(t, "judul", body.Data.Title, "Moon Rocket")
	mustEqual(t, "isi", body.Data.Content, "Aira flies to the moon.")

	var saved models.Story
	if err := e.db.First(&saved, body.Data.ID).Error; err != nil {
		t.Fatalf("cerita tidak tersimpan: %v", err)
	}
	mustEqual(t, "mood tersimpan", saved.Mood, "Calm, Brave")
	mustEqual(t, "deskripsi tersimpan", saved.Description, "A trip to the moon")
	mustEqual(t, "durasi tersimpan", saved.LengthMinutes, 5)
	mustEqual(t, "tema tersimpan", saved.Theme, "Adventure")

	listed := decodeList(t, e.listStories(t, "uid-1", childQuery(child, "")))
	mustEqual(t, "cerita baru langsung ada di playlist", len(listed.Stories), 1)
}

func TestGenerateStory_OnlyBasicEnglishIsSupported(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")

	calls := 0
	var received map[string]any
	fakeAI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		_, _ = w.Write([]byte(`{"status":"success","title":"T","response":"Once."}`))
	})
	payload := func(language string) map[string]any {
		return map[string]any{"child_id": child.ID, "character_name": "Aira", "theme": "Daily", "mood": "Calm", "length_minutes": 3, "language": language}
	}

	t.Run("bahasa campuran ditolak dan AI tidak dipanggil", func(t *testing.T) {
		for _, language := range []string{"Mixed (EN + ID)", "Bahasa Indonesia", "basic english"} {
			mustEqual(t, "status untuk "+language, e.call(t, "uid-1", http.MethodPost, "/api/story/generate", payload(language)).Code, http.StatusBadRequest)
		}
		mustEqual(t, "AI tidak dipanggil", calls, 0)
	})

	t.Run("Basic English diterima, dan bahasa kosong menjadi Basic English", func(t *testing.T) {
		for _, language := range []string{"Basic English", "", "  Basic English  "} {
			mustEqual(t, "status untuk '"+language+"'", e.call(t, "uid-1", http.MethodPost, "/api/story/generate", payload(language)).Code, http.StatusOK)
			mustEqual(t, "yang dikirim ke AI", received["language"], any("Basic English"))
		}
		var saved models.Story
		e.db.Order("id DESC").First(&saved)
		mustEqual(t, "yang tersimpan", saved.Language, "Basic English")
	})
}

func TestGenerateStory_NothingIsSavedWhenTheAIServiceFails(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	payload := map[string]any{"child_id": child.ID, "character_name": "Aira", "theme": "Daily", "mood": "Calm", "length_minutes": 3}

	fakeAI(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
	mustEqual(t, "AI menjawab 500", e.call(t, "uid-1", http.MethodPost, "/api/story/generate", payload).Code, http.StatusInternalServerError)

	fakeAI(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) })
	mustEqual(t, "AI menjawab bukan JSON", e.call(t, "uid-1", http.MethodPost, "/api/story/generate", payload).Code, http.StatusInternalServerError)

	var count int64
	e.db.Model(&models.Story{}).Count(&count)
	mustEqual(t, "tidak ada cerita tersimpan", count, int64(0))
}

func TestGenerateAudio_RecordsTheVoiceAndRejectsUnknownVoices(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
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
