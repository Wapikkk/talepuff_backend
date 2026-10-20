package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeVerifier map[string]*Identity

func (f fakeVerifier) Verify(_ context.Context, token string) (*Identity, error) {
	if identity, ok := f[token]; ok {
		return identity, nil
	}
	return nil, errors.New("token tidak dikenal")
}

func serve(t *testing.T, authorization string) (*httptest.ResponseRecorder, *Identity) {
	t.Helper()
	var seen *Identity
	r := gin.New()
	r.GET("/me", Middleware(fakeVerifier{"good": {UID: "uid-1", Email: "a@example.com"}}), func(c *gin.Context) {
		seen = IdentityFrom(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec, seen
}

func TestMiddleware_ValidTokenPassesAndExposesTheIdentity(t *testing.T) {
	rec, identity := serve(t, "Bearer good")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, seharusnya 200", rec.Code)
	}
	if identity == nil || identity.UID != "uid-1" || identity.Email != "a@example.com" {
		t.Errorf("identitas salah: %+v", identity)
	}
}

func TestMiddleware_RejectsMissingOrMalformedHeaders(t *testing.T) {
	for _, header := range []string{"", "good", "Token good", "Basic good", "Bearer", "Bearer   "} {
		rec, identity := serve(t, header)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, seharusnya 401", header, rec.Code)
		}
		if identity != nil {
			t.Errorf("header %q: handler tidak boleh dijalankan", header)
		}
	}
}

func TestMiddleware_RejectsAnInvalidToken(t *testing.T) {
	rec, identity := serve(t, "Bearer forged")
	if rec.Code != http.StatusUnauthorized || identity != nil {
		t.Errorf("status %d, identitas %+v", rec.Code, identity)
	}
}

func TestMiddleware_SchemeIsCaseInsensitive(t *testing.T) {
	if rec, _ := serve(t, "bearer good"); rec.Code != http.StatusOK {
		t.Errorf("status %d, seharusnya 200", rec.Code)
	}
}

func TestMiddleware_RejectsAVerifierThatReturnsNoUid(t *testing.T) {
	r := gin.New()
	r.GET("/me", Middleware(fakeVerifier{"empty": {UID: ""}}), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer empty")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d, seharusnya 401: identitas tanpa uid tidak boleh lolos", rec.Code)
	}
}

func TestNewFirebaseVerifier_RequiresAProjectID(t *testing.T) {
	if _, err := NewFirebaseVerifier(context.Background(), "  "); err == nil {
		t.Error("project ID kosong seharusnya ditolak saat startup")
	}
}
