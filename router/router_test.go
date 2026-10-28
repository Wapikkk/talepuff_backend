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
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"talepuff_backend/auth"
	"talepuff_backend/models"
	"talepuff_backend/totem"

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
func newTestEnv(t *testing.T) *env { return newTestEnvWithHub(t, nil) }

func newTestEnvWithHub(t *testing.T, hub totem.Hub) *env {
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

	if err := db.AutoMigrate(&models.User{}, &models.Child{}, &models.Story{}, &models.StoryJob{}, &models.Feedback{}, &models.Device{}, &models.StoryPlay{}); err != nil {
		t.Fatalf("migrasi gagal: %v", err)
	}
	if err := db.Exec("TRUNCATE TABLE story_plays, devices, feedbacks, story_jobs, stories, children, users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("gagal mengosongkan tabel tes: %v", err)
	}

	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &env{h: New(db, fakeVerifier{}, hub), db: db}
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

func (e *env) register(t *testing.T, uid, childName string) *httptest.ResponseRecorder {
	t.Helper()
	return e.call(t, uid, http.MethodPost, "/api/register", map[string]any{
		"firebase_uid": uid,
		"child_name":   childName,
		"age":          3,
		"gender":       "Girl",
		"interests":    []string{"Space"},
	})
}

type childJSON struct {
	ID              uint   `json:"ID"`
	Name            string `json:"name"`
	Age             int    `json:"age"`
	ProfilePhotoURL string `json:"profile_photo_url"`
	MascotID        string `json:"mascot_id"`
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
		{http.MethodDelete, "/api/child/1/photo"},
		{http.MethodPut, "/api/child/1/mascot"},
		{http.MethodPost, "/api/story/generate"},
		{http.MethodPost, "/api/audio/generate_audio"},
		{http.MethodPost, "/api/feedback"},
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

	mustEqual(t, "status register", e.register(t, "uid-1", "Aira").Code, http.StatusOK)

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
	e.register(t, "uid-1", "Aira")

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
		mustEqual(t, "status untuk "+name, e.register(t, "uid-1", name).Code, http.StatusBadRequest)
	}
	_, status := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "tidak ada profil yang tersimpan", status, http.StatusNotFound)
}

// ---------- membaca profil ----------

func TestGetChild_OnlyTheOwnerCanReadIt(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")

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
	e.register(t, "uid-1", "Aira")
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
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")

	rec := e.call(t, "uid-2", http.MethodPut, "/api/child/update-name/"+itoa(victim.ID), map[string]string{"name": "Hacked"})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
	after, _ := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "nama korban tidak berubah", after.Name, "Aira")
}

// ---------- maskot ----------

func TestMascot(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	path := "/api/child/" + itoa(child.ID) + "/mascot"

	t.Run("profil baru memakai Harimau", func(t *testing.T) {
		mustEqual(t, "maskot bawaan", child.MascotID, "Harimau")
	})

	t.Run("semua maskot yang tersedia bisa dipilih", func(t *testing.T) {
		for _, mascot := range []string{"OrangUtan", "Badak", "Burung", "Harimau"} {
			mustEqual(t, "status "+mascot, e.call(t, "uid-1", http.MethodPut, path, map[string]string{"mascot_id": mascot}).Code, http.StatusOK)
			saved, _ := e.child(t, "uid-1", "uid-1")
			mustEqual(t, "tersimpan "+mascot, saved.MascotID, mascot)
		}
	})

	t.Run("maskot yang tidak dikenal ditolak dan pilihan sebelumnya tidak berubah", func(t *testing.T) {
		e.call(t, "uid-1", http.MethodPut, path, map[string]string{"mascot_id": "Badak"})
		for _, bad := range []string{"", "Naga", "badak", "Harimau; DROP TABLE children"} {
			mustEqual(t, "status untuk "+bad, e.call(t, "uid-1", http.MethodPut, path, map[string]string{"mascot_id": bad}).Code, http.StatusBadRequest)
		}
		saved, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "tetap Badak", saved.MascotID, "Badak")
	})

	t.Run("pengguna lain tidak bisa mengubah maskot anak orang lain", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodPut, path, map[string]string{"mascot_id": "Burung"}).Code, http.StatusNotFound)
		saved, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "maskot korban tidak berubah", saved.MascotID, "Badak")
	})

	t.Run("id bukan angka 400 dan id yang tidak ada 404", func(t *testing.T) {
		mustEqual(t, "bukan angka", e.call(t, "uid-1", http.MethodPut, "/api/child/abc/mascot", map[string]string{"mascot_id": "Badak"}).Code, http.StatusBadRequest)
		mustEqual(t, "tidak ada", e.call(t, "uid-1", http.MethodPut, "/api/child/9999/mascot", map[string]string{"mascot_id": "Badak"}).Code, http.StatusNotFound)
	})
}

// ---------- email tidak lagi disimpan di backend ----------
//
// Dulu email disimpan di tabel users tetapi tidak pernah dibaca siapa pun, dan menjadi usang
// begitu pengguna mengganti email di Firebase (akibatnya email lamanya tetap "terpakai" dan
// pendaftar baru dengan email itu ditolak 409). Sumber kebenaran email sekarang hanya Firebase.

func TestRegister_DoesNotNeedAnEmail(t *testing.T) {
	e := newTestEnv(t)

	rec := e.call(t, "uid-1", http.MethodPost, "/api/register", map[string]any{"child_name": "Aira", "age": 3})

	mustEqual(t, "tanpa email", rec.Code, http.StatusOK)
}

// APK yang sudah terpasang masih mengirim email; itu tidak boleh membuat pendaftaran gagal.
func TestRegister_ToleratesALegacyClientThatStillSendsAnEmail(t *testing.T) {
	e := newTestEnv(t)

	rec := e.call(t, "uid-1", http.MethodPost, "/api/register", map[string]any{
		"email": "a@example.com", "child_name": "Aira", "age": 3,
	})

	mustEqual(t, "klien lama", rec.Code, http.StatusOK)
}

func TestUpdateUserEmailEndpointIsGone(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")

	rec := e.call(t, "uid-1", http.MethodPut, "/api/user/update-email/uid-1", map[string]string{"email": "new@example.com"})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
}

// ---------- foto profil ----------

var (
	jpegHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	pngHeader  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
)

// photoBody membuat unggahan multipart; [content] adalah isi berkas dan [filename] nama dari klien.
func photoBody(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func withJPEG(n int) []byte {
	content := make([]byte, n)
	copy(content, jpegHeader)
	return content
}

// chdirTemp memindahkan direktori kerja ke folder sementara dengan uploads/profiles di dalamnya,
// karena handler menulis ke ./uploads relatif terhadap direktori kerja.
func chdirTemp(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("uploads/profiles", os.ModePerm); err != nil {
		t.Fatal(err)
	}
}

func uploadedFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("uploads/profiles")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func (e *env) upload(t *testing.T, uid string, childID uint, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := photoBody(t, filename, content)
	return e.raw(t, uid, http.MethodPost, "/api/child/upload-photo/"+itoa(childID), body, contentType)
}

func TestUploadPhoto(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	t.Run("pengguna lain ditolak dan tidak ada berkas yang tersimpan", func(t *testing.T) {
		mustEqual(t, "status", e.upload(t, "uid-2", victim.ID, "me.jpg", withJPEG(100)).Code, http.StatusNotFound)
		mustEqual(t, "jumlah berkas", len(uploadedFiles(t)), 0)
	})

	t.Run("pemilik berhasil dan URL tersimpan", func(t *testing.T) {
		rec := e.upload(t, "uid-1", victim.ID, "me.jpg", withJPEG(100))
		mustEqual(t, "status", rec.Code, http.StatusOK)

		files := uploadedFiles(t)
		mustEqual(t, "jumlah berkas", len(files), 1)
		saved, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "URL foto di profil", saved.ProfilePhotoURL, "/uploads/profiles/"+files[0])
	})

	t.Run("id bukan angka dijawab 400", func(t *testing.T) {
		body, contentType := photoBody(t, "me.jpg", withJPEG(100))
		mustEqual(t, "status", e.raw(t, "uid-1", http.MethodPost, "/api/child/upload-photo/abc", body, contentType).Code, http.StatusBadRequest)
	})
}

func TestUploadPhoto_OnlyRealImagesAreAccepted(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	t.Run("berkas teks yang diberi nama .jpg ditolak (415)", func(t *testing.T) {
		mustEqual(t, "status", e.upload(t, "uid-1", child.ID, "photo.jpg", []byte("<?php echo 'not an image'; ?>")).Code, http.StatusUnsupportedMediaType)
		mustEqual(t, "tidak ada berkas tersimpan", len(uploadedFiles(t)), 0)
	})

	t.Run("jenis lain seperti GIF dan PDF ditolak", func(t *testing.T) {
		for name, content := range map[string][]byte{"a.gif": []byte("GIF89a......"), "a.pdf": []byte("%PDF-1.4 ......")} {
			mustEqual(t, "status "+name, e.upload(t, "uid-1", child.ID, name, content).Code, http.StatusUnsupportedMediaType)
		}
	})

	t.Run("PNG diterima, dan ekstensi mengikuti ISI berkas, bukan nama kiriman", func(t *testing.T) {
		mustEqual(t, "status", e.upload(t, "uid-1", child.ID, "lies.jpg", append(append([]byte{}, pngHeader...), make([]byte, 50)...)).Code, http.StatusOK)
		files := uploadedFiles(t)
		mustEqual(t, "jumlah berkas", len(files), 1)
		mustEqual(t, "ekstensi", filepath.Ext(files[0]), ".png")
	})
}

func TestUploadPhoto_RejectsAnOversizedFile(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	mustEqual(t, "status", e.upload(t, "uid-1", child.ID, "big.jpg", withJPEG(5<<20+1024)).Code, http.StatusRequestEntityTooLarge)
	mustEqual(t, "tidak ada berkas tersimpan", len(uploadedFiles(t)), 0)

	mustEqual(t, "di bawah batas diterima", e.upload(t, "uid-1", child.ID, "ok.jpg", withJPEG(1<<20)).Code, http.StatusOK)
}

func TestUploadPhoto_ClientFilenameIsNeverUsed(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	mustEqual(t, "status", e.upload(t, "uid-1", child.ID, "../../evil.jpg", withJPEG(100)).Code, http.StatusOK)

	files := uploadedFiles(t)
	mustEqual(t, "jumlah berkas di folder profil", len(files), 1)
	if !strings.HasPrefix(files[0], "child-") {
		t.Errorf("nama berkas %q seharusnya dibuat server", files[0])
	}
	if _, err := os.Stat("evil.jpg"); err == nil {
		t.Error("berkas lolos keluar dari folder profil")
	}
}

// Foto profil dilayani publik di /uploads, jadi nama berkasnya adalah satu-satunya "rahasia":
// harus acak (128-bit), bukan ID anak + cap waktu yang bisa ditebak.
var photoNamePattern = regexp.MustCompile(`^child-\d+-[0-9a-f]{32}\.jpg$`)

func TestUploadPhoto_FilenameIsRandomNotPredictable(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		mustEqual(t, "status", e.upload(t, "uid-1", child.ID, "me.jpg", withJPEG(100)).Code, http.StatusOK)
		files := uploadedFiles(t)
		mustEqual(t, "hanya satu berkas tersisa", len(files), 1)
		if !photoNamePattern.MatchString(files[0]) {
			t.Fatalf("nama berkas %q tidak berbentuk child-<id>-<32 hex>.jpg", files[0])
		}
		if seen[files[0]] {
			t.Fatalf("nama berkas %q muncul dua kali", files[0])
		}
		seen[files[0]] = true
	}
}

func TestUploadPhoto_ReplacingThePhotoDeletesTheOldFile(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)

	e.upload(t, "uid-1", child.ID, "first.jpg", withJPEG(100))
	first := uploadedFiles(t)
	e.upload(t, "uid-1", child.ID, "second.jpg", withJPEG(200))
	second := uploadedFiles(t)

	mustEqual(t, "hanya satu berkas tersisa", len(second), 1)
	if first[0] == second[0] {
		t.Error("berkas baru seharusnya memakai nama yang berbeda")
	}
	saved, _ := e.child(t, "uid-1", "uid-1")
	mustEqual(t, "profil menunjuk berkas baru", saved.ProfilePhotoURL, "/uploads/profiles/"+second[0])
}

func TestDeletePhoto(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	child, _ := e.child(t, "uid-1", "uid-1")
	chdirTemp(t)
	e.upload(t, "uid-1", child.ID, "me.jpg", withJPEG(100))
	path := "/api/child/" + itoa(child.ID) + "/photo"

	t.Run("pengguna lain tidak bisa menghapus foto orang lain", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-2", http.MethodDelete, path, nil).Code, http.StatusNotFound)
		mustEqual(t, "berkas masih ada", len(uploadedFiles(t)), 1)
		still, _ := e.child(t, "uid-1", "uid-1")
		if still.ProfilePhotoURL == "" {
			t.Error("URL foto korban terhapus")
		}
	})

	t.Run("pemilik menghapus: berkas hilang dan URL dikosongkan", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, path, nil).Code, http.StatusOK)
		mustEqual(t, "berkas di disk", len(uploadedFiles(t)), 0)
		after, _ := e.child(t, "uid-1", "uid-1")
		mustEqual(t, "URL foto", after.ProfilePhotoURL, "")
	})

	t.Run("menghapus lagi tetap aman (tidak ada foto)", func(t *testing.T) {
		mustEqual(t, "status", e.call(t, "uid-1", http.MethodDelete, path, nil).Code, http.StatusOK)
	})

	t.Run("id bukan angka dijawab 400 dan id yang tidak ada 404", func(t *testing.T) {
		mustEqual(t, "bukan angka", e.call(t, "uid-1", http.MethodDelete, "/api/child/abc/photo", nil).Code, http.StatusBadRequest)
		mustEqual(t, "tidak ada", e.call(t, "uid-1", http.MethodDelete, "/api/child/9999/photo", nil).Code, http.StatusNotFound)
	})
}

// ---------- cerita dan audio ----------

func TestGenerateStory_CannotUseSomeoneElsesChild(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
	victim, _ := e.child(t, "uid-1", "uid-1")

	rec := e.call(t, "uid-2", http.MethodPost, "/api/story/generate", map[string]any{
		"child_id": victim.ID, "character_name": "Aira", "theme": "Daily", "mood": "Calm", "length_minutes": 3, "voice_tone": "Warm",
	})

	mustEqual(t, "status", rec.Code, http.StatusNotFound)
	var stories int64
	e.db.Model(&models.Story{}).Count(&stories)
	mustEqual(t, "tidak ada cerita yang dibuat", stories, int64(0))
}

func TestGenerateAudio_CannotUseSomeoneElsesStory(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")
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

// ---------- feedback ----------

func feedback(category, message string) map[string]string {
	return map[string]string{"category": category, "message": message, "app_version": "1.0.0+1"}
}

func TestFeedback_IsStoredForTheAuthenticatedUser(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")

	rec := e.call(t, "uid-1", http.MethodPost, "/api/feedback", feedback("Bug", "  The audio stops after a minute.  "))
	mustEqual(t, "status", rec.Code, http.StatusCreated)

	var saved models.Feedback
	if err := e.db.First(&saved).Error; err != nil {
		t.Fatalf("feedback tidak tersimpan: %v", err)
	}
	mustEqual(t, "kategori dirapikan", saved.Category, "bug")
	mustEqual(t, "pesan dirapikan", saved.Message, "The audio stops after a minute.")
	mustEqual(t, "versi aplikasi", saved.AppVersion, "1.0.0+1")

	var user models.User
	e.db.Where("firebase_uid = ?", "uid-1").First(&user)
	mustEqual(t, "pemilik adalah pengguna dari token", saved.UserID, user.ID)
}

func TestFeedback_RejectsInvalidInput(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")

	bad := []map[string]string{
		feedback("complaint", "A valid message here"),
		feedback("", "A valid message here"),
		feedback("bug", ""),
		feedback("bug", "   "),
		feedback("bug", "hi"),
		feedback("bug", strings.Repeat("a", 1001)),
		feedback("bug", "bad \x00 control character"),
	}
	for _, body := range bad {
		mustEqual(t, "status untuk "+body["category"]+"/"+body["message"], e.call(t, "uid-1", http.MethodPost, "/api/feedback", body).Code, http.StatusBadRequest)
	}

	var count int64
	e.db.Model(&models.Feedback{}).Count(&count)
	mustEqual(t, "tidak ada yang tersimpan", count, int64(0))
}

func TestFeedback_ADodgyAppVersionIsDroppedNotRejected(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")

	body := map[string]string{"category": "idea", "message": "Please add more stories", "app_version": "<script>alert(1)</script>"}
	mustEqual(t, "status", e.call(t, "uid-1", http.MethodPost, "/api/feedback", body).Code, http.StatusCreated)

	var saved models.Feedback
	e.db.First(&saved)
	mustEqual(t, "versi aneh dikosongkan", saved.AppVersion, "")
}

func TestFeedback_IsRateLimitedPerUser(t *testing.T) {
	e := newTestEnv(t)
	e.register(t, "uid-1", "Aira")
	e.register(t, "uid-2", "Bimo")

	for i := 0; i < 5; i++ {
		mustEqual(t, "pengiriman ke-"+itoa(uint(i+1)), e.call(t, "uid-1", http.MethodPost, "/api/feedback", feedback("idea", "Another good idea")).Code, http.StatusCreated)
	}
	mustEqual(t, "pengiriman ke-6", e.call(t, "uid-1", http.MethodPost, "/api/feedback", feedback("idea", "One more idea")).Code, http.StatusTooManyRequests)

	mustEqual(t, "pengguna lain tidak terpengaruh", e.call(t, "uid-2", http.MethodPost, "/api/feedback", feedback("idea", "My first idea")).Code, http.StatusCreated)
}

func TestFeedback_NeedsARegisteredUser(t *testing.T) {
	e := newTestEnv(t)
	mustEqual(t, "status", e.call(t, "uid-never-registered", http.MethodPost, "/api/feedback", feedback("bug", "Something is wrong")).Code, http.StatusNotFound)
}
