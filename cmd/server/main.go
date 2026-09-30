// Command server runs the NanoFaktura HTTP server (no arguments or "serve")
// and the administration commands ("backup export|import", see backup.go;
// "user reset-2fa", see user.go).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/bankimport"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/httpsec"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/scheduler"
	"github.com/qwerin/nanofaktura/internal/secret"
)

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "backup":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err = runBackup(ctx, os.Args[2:], os.Stdout, os.Stderr)
		stop()
	case len(os.Args) > 1 && os.Args[1] == "user":
		err = runUser(context.Background(), os.Args[2:], os.Stdout)
	case len(os.Args) > 1 && os.Args[1] != "serve":
		err = errors.New("unknown command " + os.Args[1] + " (serve | backup | user)")
	default:
		err = run()
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	gdb, err := db.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		return err
	}
	if err := db.Migrate(gdb); err != nil {
		return err
	}

	key, generated, err := secret.LoadOrCreateKey(cfg.SecretKey, cfg.DataDir)
	if err != nil {
		return err
	}
	if generated {
		slog.Warn("NANOFAKTURA_SECRET_KEY is not set; generated a key for stored secrets — back it up with the database",
			"file", filepath.Join(cfg.DataDir, secret.KeyFile))
	}
	secrets, err := secret.New(key)
	if err != nil {
		return err
	}

	// one Fio client for requests and jobs, so its 30 s per-token limit is shared
	deps := api.Deps{Secrets: secrets, Fio: bankimport.NewFioClient(cfg.FioURL)}
	handler, _ := api.New(gdb, cfg, deps)
	if cfg.StaticDir != "" {
		// security headers (CSP …) for the SPA too; the API sets its own
		handler = httpsec.Middleware(httpsec.Options{TrustedProxies: cfg.TrustedProxies, PublicHTTPS: cfg.PublicHTTPS()})(
			withSPA(handler, cfg.StaticDir))
	}
	warnInsecureSetup(gdb, cfg)
	// Uploads and imports extend their read deadline per operation (huma
	// BodyReadTimeout), streamed exports their write deadline.
	srv := &http.Server{
		Addr: cfg.ListenAddr, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 2 * time.Minute,
		WriteTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// background jobs: the scheduler ticks every minute (webhook deliveries);
	// the other jobs declare Every (hourly …). Stopped by ctx on shutdown.
	sched := scheduler.New(nil, time.Minute, slog.Default())
	sched.Register(api.Jobs(gdb, cfg, deps)...)
	schedDone := make(chan struct{})
	go func() { sched.Start(ctx); close(schedDone) }()
	defer func() { stop(); <-schedDone }()

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "db", cfg.DBDriver, "static", cfg.StaticDir)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// warnInsecureSetup logs configuration that is fine for development but
// risky in production.
func warnInsecureSetup(gdb *gorm.DB, cfg config.Config) {
	if cfg.SMTPHost == "" {
		slog.Warn("NANOFAKTURA_SMTP_HOST is not set: e-mails (including invitation links) are only written to the log")
	}
	if !cfg.PublicHTTPS() {
		slog.Warn("NANOFAKTURA_PUBLIC_URL is not https: fine locally, but in production serve the app over HTTPS", "public_url", cfg.PublicURL)
	}
	var n int64
	if err := gdb.Model(&model.User{}).Count(&n).Error; err == nil && n == 0 && cfg.SetupToken == "" {
		slog.Warn("no user registered yet and NANOFAKTURA_SETUP_TOKEN is not set: anyone reaching the server can register first")
	}
}

// withSPA serves /api/* from the API and everything else from dir, falling
// back to index.html for unknown paths (client-side routing).
func withSPA(apiHandler http.Handler, dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}
