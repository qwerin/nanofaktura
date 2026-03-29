package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/handler"
	"github.com/qwerin/nanofaktura/internal/models"
)

// newTestAPIChi vrátí chi router s plným routing stackem a in-memory DB (single-user).
func newTestAPIChi(t *testing.T) http.Handler {
	t.Helper()
	h, _ := newTestAPIWithDB(t)
	return h
}

// newTestAPIWithDB vrátí router i DB — pro testy které potřebují přímý přístup k DB.
func newTestAPIWithDB(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Výchozí Settings (ID=1) pro single-user testy
	s := models.DefaultSettings()
	db.Create(&s)

	var multiUser, initialized atomic.Bool
	initialized.Store(true)

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "1.0.0"))
	handler.RegisterHealth(api, db, &multiUser, &initialized)
	handler.RegisterInvoice(api, r, db)
	handler.RegisterSubject(api, db)
	handler.RegisterSettings(api, db)
	handler.RegisterUsers(api, db, &multiUser)
	handler.RegisterAuth(api, r, db, config.AppConfig{SessionTTL: 24}, &multiUser, &initialized)
	return r, db
}

// newTestAPISetup vrátí router s initialized=false pro testování setup/init endpointu.
func newTestAPISetup(t *testing.T) http.Handler {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	var multiUser, initialized atomic.Bool
	// initialized=false → setup endpoint je dostupný

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "1.0.0"))
	handler.RegisterHealth(api, db, &multiUser, &initialized)
	handler.RegisterAuth(api, r, db, config.AppConfig{SessionTTL: 24}, &multiUser, &initialized)
	return r
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// decodeBody dekóduje JSON tělo odpovědi do dst.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	raw := w.Body.Bytes()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode body: %v\nraw: %s", err, raw)
	}
}

func idStr(id uint) string {
	return fmt.Sprintf("%d", id)
}
