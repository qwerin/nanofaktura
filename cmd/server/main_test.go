package main

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestWithSPA(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>index</html>"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644))
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	must(os.WriteFile(secret, []byte("top secret"), 0o644))
	t.Cleanup(func() { os.Remove(secret) })

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "api:"+r.URL.Path) })
	h := withSPA(api, dir)
	for path, want := range map[string]string{
		"/api":                "api:/api",
		"/api/auth/me":        "api:/api/auth/me",
		"/assets/app.js":      "console.log(1)",
		"/":                   "<html>index</html>",
		"/a/firma/invoices/5": "<html>index</html>", // client-side route
		"/assets":             "<html>index</html>", // directory → index, no listing
		"/apixyz":             "<html>index</html>",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://x/", nil)
		req.URL.Path = path
		h.ServeHTTP(rec, req)
		if got := rec.Body.String(); got != want || rec.Code != http.StatusOK {
			t.Errorf("%s: %d %q, want %q", path, rec.Code, got, want)
		}
	}
	// no traversal out of dir (net/http answers 400 for ".." segments)
	for _, path := range []string{"/../secret.txt", "/assets/../../secret.txt"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://x/", nil)
		req.URL.Path = path
		h.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "top secret") {
			t.Errorf("%s: file outside the static dir served", path)
		}
	}
}

func TestWarnInsecureSetup(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	warnInsecureSetup(gdb, config.Config{PublicURL: "http://localhost:8080"})
	out := buf.String()
	for _, w := range []string{"NANOFAKTURA_SMTP_HOST is not set", "NANOFAKTURA_PUBLIC_URL is not https", "NANOFAKTURA_SETUP_TOKEN is not set"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing warning %q in:\n%s", w, out)
		}
	}

	buf.Reset()
	gdb.Create(&model.User{Email: "a@example.cz", Name: "A", PasswordHash: "x"})
	warnInsecureSetup(gdb, config.Config{PublicURL: "https://faktury.example.cz", SMTPHost: "smtp.example.cz"})
	if buf.Len() != 0 {
		t.Errorf("secure setup logged warnings:\n%s", buf.String())
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// TestRunServesAndShutsDown starts the real server (API + SPA), checks a
// request and stops it with SIGTERM (caught by run's signal context).
func TestRunServesAndShutsDown(t *testing.T) {
	dir := t.TempDir()
	static := filepath.Join(dir, "dist")
	if err := os.MkdirAll(static, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(static, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	addr := freePort(t)
	t.Setenv("NANOFAKTURA_LISTEN_ADDR", addr)
	t.Setenv("NANOFAKTURA_DB_DRIVER", "sqlite")
	t.Setenv("NANOFAKTURA_DB_DSN", filepath.Join(dir, "nf.db"))
	t.Setenv("NANOFAKTURA_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("NANOFAKTURA_STATIC_DIR", static)
	t.Setenv("NANOFAKTURA_SECRET_KEY", "")

	errc := make(chan error, 1)
	go func() { errc <- run() }()
	var res *http.Response
	var err error
	for range 200 {
		if res, err = http.Get("http://" + addr + "/api/health"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health: %d", res.StatusCode)
	}
	res, err = http.Get("http://" + addr + "/a/firma")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "<html>spa</html>" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src") {
		t.Fatalf("spa: %q %v", body, res.Header)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "secret.key")); err != nil {
		t.Fatalf("secret key not generated: %v", err)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down")
	}

	// port already taken → run returns the listen error
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	t.Setenv("NANOFAKTURA_LISTEN_ADDR", ln.Addr().String())
	if err := run(); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("busy port: %v", err)
	}
}
