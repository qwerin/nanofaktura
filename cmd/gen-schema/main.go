// gen-schema vypíše OpenAPI JSON schéma na stdout.
// Používá se pro generování TypeScript typů: make gen-types
package main

import (
	"encoding/json"
	"log"
	"os"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/handler"
)


func main() {
	db, err := database.Open(":memory:")
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	var multiUser atomic.Bool
	var initialized atomic.Bool

	apiRouter := chi.NewRouter()
	apiRouter.Use(auth.ConditionalAuth(db, &multiUser))
	api := humachi.New(apiRouter, huma.DefaultConfig("NanoFaktura API", "1.0.0"))

	handler.RegisterHealth(api, db, &multiUser, &initialized)
	handler.RegisterAuth(api, apiRouter, db, config.AppConfig{SessionTTL: 720}, &multiUser, &initialized)
	handler.RegisterInvoice(api, apiRouter, db)
	handler.RegisterSubject(api, db)
	handler.RegisterAres(api)
	handler.RegisterSettings(api, db)
	handler.RegisterPriceItems(api, db)
	handler.RegisterUsers(api, db, &multiUser)

	schema := api.OpenAPI()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		log.Fatalf("encode: %v", err)
	}
}
