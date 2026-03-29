package handler

import (
	"context"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/models"
)

type HealthOutput struct {
	Body struct {
		Status      string `json:"status"      doc:"Stav služby" example:"ok"`
		Version     string `json:"version"     doc:"Verze API" example:"1.0.0"`
		MultiUser   bool   `json:"multi_user"  doc:"Je zapnutý multi-user mód"`
		Initialized bool   `json:"initialized" doc:"Byl dokončen první setup"`
	}
}

// RegisterHealth registruje GET /health endpoint.
// Frontend čte multi_user a initialized k rozhodnutí o zobrazení setup/login screenu.
func RegisterHealth(api huma.API, db *gorm.DB, multiUser *atomic.Bool, initialized *atomic.Bool) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      "GET",
		Path:        "/health",
		Summary:     "Zdravotní stav služby",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = "1.0.0"
		out.Body.MultiUser = multiUser.Load()

		// initialized čteme z DB aby byl vždy aktuální (SystemConfig ID=1)
		var syscfg models.SystemConfig
		if err := db.First(&syscfg, 1).Error; err == nil {
			out.Body.Initialized = syscfg.Initialized
		} else {
			out.Body.Initialized = initialized.Load()
		}

		return out, nil
	})
}
