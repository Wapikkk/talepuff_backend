package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"talepuff_backend/auth"
	"talepuff_backend/models"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func init() { gin.SetMode(gin.TestMode) }

// fakeVerifier mengenali token "token-<uid>" sebagai pengguna <uid>.
type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, token string) (*auth.Identity, error) {
	uid, ok := strings.CutPrefix(token, "token-")
	if !ok || uid == "" {
		return nil, errors.New("token tidak dikenal")
	}
	return &auth.Identity{UID: uid, Email: uid + "@example.com"}, nil
}

type env struct {
	h  http.Handler
	db *gorm.DB
}

// newTestEnv memakai database tes TERPISAH (TEST_DB_URL). Menolak berjalan bila nama database
// tidak berakhiran "_test", supaya tes yang mengosongkan tabel tidak pernah menyentuh data asli.
func newTestEnv(t *testing.T) *env {
	t.Helper()
	_ = godotenv.Load("../.env")
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL belum diatur: tes backend dilewati")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("tidak bisa terhubung ke database tes: %v", err)
	}

	var name string
	if err := db.Raw("SELECT current_database()").Scan(&name).Error; err != nil {
		t.Fatalf("gagal membaca nama database: %v", err)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatalf("menolak menjalankan tes di database %q: namanya harus berakhiran _test", name)
	}

	if err := db.AutoMigrate(&models.User{}, &models.Child{}, &models.Story{}); err != nil {
		t.Fatalf("migrasi gagal: %v", err)
	}
	if err := db.Exec("TRUNCATE TABLE stories, children, users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("gagal mengosongkan tabel tes: %v", err)
	}

	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &env{h: New(db, fakeVerifier{}), db: db}
}

// raw mengirim permintaan sebagai pengguna [uid]; uid kosong = tanpa header Authorization.
func (e *env) raw(t *testing.T, uid, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if uid != "" {
		req.Header.Set("Authorization", "Bearer token-"+uid)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) call(t *testing.T, uid, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	return e.raw(t, uid, method, path, bytes.NewReader(raw), "application/json")
}

func (e *env) register(t *testing.T, uid, email, childName string) *httptest.ResponseRecorder {
	t.Helper()
	return e.call(t, uid, http.MethodPost, "/api/register", map[string]any{
		"firebase_uid": uid,
		"email":        email,
		"child_name":   childName,
		"age":          3,
		"gender":       "Girl",
		"interests":    []string{"Space"},
	})
}

type childJSON struct {
	ID   uint   `json:"ID"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (e *env) child(t *testing.T, asUID, ofUID string) (childJSON, int) {
	t.Helper()
	rec := e.call(t, asUID, http.MethodGet, "/api/child/"+ofUID, nil)
	var child childJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &child)
	return child, rec.Code
}

func mustEqual[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: dapat %v, seharusnya %v", what, got, want)
	}
}

// ---------- autentikasi ----------

func TestAuth_EveryApiRouteRequiresAValidToken(t *testing.T) {
	e := newTestEnv(t)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/register"},
		{http.MethodGet, "/api/child/uid-1"},
		{http.MethodPost, "/api/child/upload-photo/1"},
		{http.MethodPut, "/api/child/update-name/1"},
		{http.MethodPut, "/api/user/update-email/uid-1"},
		{http.MethodPost, "/api/story/generate"},
		{http.MethodPost, "/api/audio/generate_audio"},
	}
	for _, route := range routes {
		label := route.method + " " + route.path

		mustEqual(t, label+" tanpa token", e.call(t, "", route.method, route.path, map[string]string{}).Code, http.StatusUnauthorized)

		req := httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", "Bearer forged")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		mustEqual(t, label+" token palsu", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuth_ProfilePhotosRemainPublicForNow(t *testing.T) {
	e := newTestEnv(t)
	rec := e.raw(t, "", http.MethodGet, "/uploads/does-not-exist.jpg", nil, "")
	mustEqual(t, "status", rec.Code, http.StatusNotFound) // 404 dari file statis, bukan 401
}

// ---------- register ----------

func TestRegister_CreatesUserAndChild(t *testing.T) {
	e := newTestEnv(t)

	mustEqual(t, "status register", e.register(t, "uid-1", "a@example.com", "Aira").Code, http.StatusOK)

	child, status := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "status get child", status, http.StatusOK)
	mustEqual(t, "nama anak", child.Name, "Aira")
	mustEqual(t, "usia", child.Age, 3)
}

func TestRegister_UsesTheUidFromTheTokenWhenTheBodyHasNone(t *testing.T) {
	e := newTestEnv(t)

	rec := e.call(t, "uid-1", http.MethodPost, "/api/register", map[string]any{
		"email": "a@example.com", "child_name": "Aira", "age": 3,
	})
	mustEqual(t, "status", rec.Code, http.StatusOK)

	_, status := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "profil tersimpan untuk uid token", status, http.StatusOK)
}

func TestRegister_RejectsABodyUidThatIsNotTheCallers(t *testing.T) {
	e := newTestEnv(t)

	rec := e.call(t, "uid-attacker", http.MethodPost, "/api/register", map[string]any{
		"firebase_uid": "uid-victim", "email": "v@example.com", "child_name": "Aira", "age": 3,
	})
	mustEqual(t, "status", rec.Code, http.StatusForbidden)

	var users int64
	e.db.Model(&models.User{}).Count(&users)
	mustEqual(t, "tidak ada user yang dibuat", users, int64(0))
}

func TestRegister_DoesNotOverwriteAnExistingChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")

	second := e.call(t, "uid-1", http.MethodPost, "/api/register", map[string]any{
		"firebase_uid": "uid-1", "email": "a@example.com", "child_name": "Hacker", "age": 9,
	})

	mustEqual(t, "status register kedua", second.Code, http.StatusConflict)
	child, _ := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "nama anak setelah register kedua", child.Name, "Aira")
	mustEqual(t, "usia setelah register kedua", child.Age, 3)
}

func TestRegister_RejectsAnInvalidChildName(t *testing.T) {
	e := newTestEnv(t)

	for _, name := range []string{"Aira 2", "Aira!", "2Aira", strings.Repeat("A", 31)} {
		mustEqual(t, "status untuk "+name, e.register(t, "uid-1", "a@example.com", name).Code, http.StatusBadRequest)
	}
	_, status := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "tidak ada profil yang tersimpan", status, http.StatusNotFound)
}

func TestRegister_RequiresAnEmail(t *testing.T) {
	e := newTestEnv(t)
	mustEqual(t, "tanpa email", e.register(t, "uid-1", "", "Aira").Code, http.StatusBadRequest)
}

// ---------- membaca profil ----------

func TestGetChild_OnlyTheOwnerCanReadIt(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")

	_, own := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "pemilik", own, http.StatusOK)

	_, other := e.child(t, "uid-2", "uid-1")
	mustEqual(t, "pengguna lain", other, http.StatusForbidden)
}

func TestGetChild_NoProfileYetIs404(t *testing.T) {
	e := newTestEnv(t)
	_, status := e.child(t, "uid-404", "uid-404")
	mustEqual(t, "status", status, http.StatusNotFound)
}

// ---------- update nama anak ----------

func TestUpdateChildName(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	path := "/api/child/update-name/" + itoa(child.ID)

	t.Run("nama valid disimpan dan dirapikan", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodPut, path, map[string]string{"name": "  Aira Putri  "}).Code, http.StatusOK)
		updated, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "nama baru", updated.Name, "Aira Putri")
	})

	t.Run("nama tidak valid ditolak dan data tidak berubah", func(t *testing.T) {
		for _, name := range []string{"", "   ", "Aira 2", "Aira!", strings.Repeat("A", 31)} {
			mustEqual(t, "status untuk "+name, e.call(t, "uid-1", http.MethodPut, path, map[string]string{"name": name}).Code, http.StatusBadRequest)
		}
		unchanged, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "nama tetap", unchanged.Name, "Aira Putri")
	})

	t.Run("id yang tidak ada dijawab 404", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodPut, "/api/child/update-name/9999", map[string]string{"name": "Bimo"}).Code, http.StatusNotFound)
	})

	t.Run("id bukan angka dijawab 400", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodPut, "/api/child/update-name/abc", map[string]string{"name": "Bimo"}).Code, http.StatusBadRequest)
	})
}

func TestUpdateChildName_CannotRenameSomeoneElsesChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")

	rec := e.call(t, "uid-2", http.MethodPut, "/api/child/update-name/"+itoa(victim.ID), map[string]string{"name": "Hacked"})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
	after, _ := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "nama korban tidak berubah", after.Name, "Aira")
}

// ---------- update email ----------

func TestUpdateUserEmail(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")

	t.Run("email valid disimpan", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodPut, "/api/user/update-email/uid-1", map[string]string{"email": "new@example.com"}).Code, http.StatusOK)
	})

	t.Run("email tidak valid ditolak", func(t *testing.T) {
		for _, email := range []string{"", "plain", "a@b", "@b.com", "a b@c.com"} {
			mustEqual(t, "status untuk "+email, e.call(t, "uid-1", http.MethodPut, "/api/user/update-email/uid-1", map[string]string{"email": email}).Code, http.StatusBadRequest)
		}
	})

	t.Run("email yang sudah dipakai akun lain dijawab 409", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodPut, "/api/user/update-email/uid-1", map[string]string{"email": "b@example.com"}).Code, http.StatusConflict)
	})

	t.Run("pengguna lain tidak boleh mengubah email orang lain", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodPut, "/api/user/update-email/uid-1", map[string]string{"email": "evil@example.com"}).Code, http.StatusForbidden)
	})
}

// ---------- foto profil ----------

func photoBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "me.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake-image-bytes"))
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func TestUploadPhoto(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")
	path := "/api/child/upload-photo/" + itoa(victim.ID)

	t.Chdir(t.TempDir()) // berkas unggahan ditulis ke ./uploads relatif terhadap direktori kerja
	if err := os.MkdirAll("uploads/profiles", os.ModePerm); err != nil {
		t.Fatal(err)
	}

	t.Run("pengguna lain ditolak dan tidak ada berkas yang tersimpan", func(t *testing.T) {
		body, contentType := photoBody(t)
		mustEqual(t, "status", e.raw(t, "uid-2", http.MethodPost, path, body, contentType).Code, http.StatusNotFound)

		entries, _ := os.ReadDir("uploads/profiles")
		mustEqual(t, "jumlah berkas", len(entries), 0)
	})

	t.Run("pemilik berhasil", func(t *testing.T) {
		body, contentType := photoBody(t)
		mustEqual(t, "status", e.raw(t, "uid-1", http.MethodPost, path, body, contentType).Code, http.StatusOK)

		entries, _ := os.ReadDir("uploads/profiles")
		mustEqual(t, "jumlah berkas", len(entries), 1)
	})

	t.Run("id bukan angka dijawab 400", func(t *testing.T) {
		body, contentType := photoBody(t)
		mustEqual(t, "status", e.raw(t, "uid-1", http.MethodPost, "/api/child/upload-photo/abc", body, contentType).Code, http.StatusBadRequest)
	})
}

// ---------- cerita dan audio ----------

func TestGenerateStory_CannotUseSomeoneElsesChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")

	rec := e.call(t, "uid-2", http.MethodPost, "/api/story/generate", map[string]any{
		"child_id": victim.ID, "character_name": "Aira", "theme": "Daily", "mood": "Calm", "length_minutes": 3,
	})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
	var stories int64
	e.db.Model(&models.Story{}).Count(&stories)
	mustEqual(t, "tidak ada cerita yang dibuat", stories, int64(0))
}

func TestGenerateAudio_CannotUseSomeoneElsesStory(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "a@example.com", "Aira")
	e.register(t, "uid-2", "b@example.com", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")

	story := models.Story{ChildID: victim.ID, Title: "T", Content: "C"}
	if err := e.db.Create(&story).Error; err != nil {
		t.Fatal(err)
	}

	rec := e.call(t, "uid-2", http.MethodPost, "/api/audio/generate_audio", map[string]any{
		"story_id": story.ID, "voice_tone": "Warm",
	})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
}
