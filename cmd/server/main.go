// NanoFaktura – headless fakturační engine pro OSVČ.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/handler"
)

func main() {
	cfg := config.Load()

	// --- Databáze ----------------------------------------------------------
	db, err := database.Open(cfg.DBPath, cfg.DBDriver)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}

	// --- Runtime stav (aktualizován po setup/init bez restartu) -----------
	syscfg := database.EnsureSystemConfig(db)
	var multiUser atomic.Bool
	var initialized atomic.Bool
	multiUser.Store(syscfg.MultiUser)
	initialized.Store(syscfg.Initialized)

	// Pokud je inicializováno jako multi-user a existují osiřelá data → migruj
	if syscfg.MultiUser && syscfg.Initialized {
		var superAdmin auth.User
		if err := db.Where("role = ? AND is_active = true", auth.RoleSuperAdmin).
			Order("id asc").First(&superAdmin).Error; err == nil {
			database.MigrateOwnership(db, superAdmin.ID)
		}
	}

	// --- HTTP router -------------------------------------------------------
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// --- API sub-router pod /api -------------------------------------------
	// ConditionalAuth pouze na API routách — SPA soubory jsou veřejné.
	apiRouter := chi.NewRouter()
	apiRouter.Use(auth.ConditionalAuth(db, &multiUser))
	api := humachi.New(apiRouter, huma.DefaultConfig("NanoFaktura API", "1.0.0"))

	handler.RegisterHealth(api, db, &multiUser, &initialized)
	handler.RegisterAuth(api, apiRouter, db, cfg, &multiUser, &initialized)
	handler.RegisterInvoice(api, apiRouter, db)
	handler.RegisterSubject(api, db)
	handler.RegisterAres(api)
	handler.RegisterSettings(api, db)
	handler.RegisterUsers(api, db, &multiUser)

	r.Mount("/api", apiRouter)

	// --- SPA static file serving ------------------------------------------
	// Aktivuje se pouze pokud je NANOFAKTURA_STATIC_DIR nastaven (produkce).
	// Dev: frontend běží přes Vite na :5173.
	if cfg.StaticDir != "" {
		r.Handle("/*", spaHandler(cfg.StaticDir))
	}

	log.Printf("NanoFaktura listening on %s (driver=%s, static=%q)", cfg.ListenAddr, cfg.DBDriver, cfg.StaticDir)
	if err := http.ListenAndServe(cfg.ListenAddr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// spaHandler vrátí HTTP handler který servíruje SPA soubory ze staticDir.
// Existující soubory (JS, CSS, obrázky) jsou servírovány přímo.
// Neexistující cesty dostávají index.html (client-side routing).
func spaHandler(staticDir string) http.Handler {
	fs := http.FileServer(http.Dir(staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, filepath.FromSlash(r.URL.Path))
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}
