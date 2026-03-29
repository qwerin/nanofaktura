package handler

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/models"
)

// RegisterPriceItems registruje endpointy pro ceník a skladové pohyby.
func RegisterPriceItems(api huma.API, db *gorm.DB) {

	// ── Pomocná funkce: aktuální stav skladu ──────────────────────────────
	stockQty := func(ctx context.Context, itemID uint) float64 {
		var sum float64
		db.WithContext(ctx).Model(&models.StockMovement{}).
			Where("price_item_id = ?", itemID).
			Select("COALESCE(SUM(CAST(quantity AS REAL)), 0)").
			Scan(&sum)
		return sum
	}

	// ── PriceItem output s aktuálním stavem skladu ────────────────────────
	type PriceItemOut struct {
		models.PriceItem
		StockQuantity float64 `json:"stock_quantity"` // computed ze StockMovement
	}

	// ── LIST ─────────────────────────────────────────────────────────────
	type ListInput struct {
		Archived bool `query:"archived" doc:"Zahrnout archivované položky"`
	}
	type ListOutput struct {
		Body []PriceItemOut
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-price-items",
		Method:      "GET",
		Path:        "/price-items",
		Summary:     "Seznam ceníkových položek",
		Tags:        []string{"price-items"},
	}, func(ctx context.Context, in *ListInput) (*ListOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var items []models.PriceItem
		q := db.WithContext(ctx).Where("user_id = ?", userID)
		if !in.Archived {
			q = q.Where("archived = false")
		}
		if err := q.Order("name asc").Find(&items).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		out := make([]PriceItemOut, len(items))
		for i, item := range items {
			out[i] = PriceItemOut{PriceItem: item}
			if item.TrackStock {
				out[i].StockQuantity = stockQty(ctx, item.ID)
			}
		}
		return &ListOutput{Body: out}, nil
	})

	// ── CREATE ────────────────────────────────────────────────────────────
	type CreateInput struct {
		Body PriceItemInput
	}
	type CreateOutput struct {
		Body PriceItemOut
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-price-item",
		Method:        "POST",
		Path:          "/price-items",
		Summary:       "Vytvoř ceníkovou položku",
		Tags:          []string{"price-items"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *CreateInput) (*CreateOutput, error) {
		item := in.Body.toModel()
		item.UserID = auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení selhalo", err)
		}
		if err := db.WithContext(ctx).First(&item, item.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &CreateOutput{Body: PriceItemOut{PriceItem: item}}, nil
	})

	// ── GET ───────────────────────────────────────────────────────────────
	type GetInput struct {
		ID uint `path:"id" doc:"ID ceníkové položky"`
	}
	type GetOutput struct {
		Body PriceItemOut
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-price-item",
		Method:      "GET",
		Path:        "/price-items/{id}",
		Summary:     "Detail ceníkové položky",
		Tags:        []string{"price-items"},
	}, func(ctx context.Context, in *GetInput) (*GetOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var item models.PriceItem
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&item, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("položka %d nenalezena", in.ID), err)
		}
		out := PriceItemOut{PriceItem: item}
		if item.TrackStock {
			out.StockQuantity = stockQty(ctx, item.ID)
		}
		return &GetOutput{Body: out}, nil
	})

	// ── UPDATE ────────────────────────────────────────────────────────────
	type UpdateInput struct {
		ID   uint           `path:"id"`
		Body PriceItemInput
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-price-item",
		Method:      "PUT",
		Path:        "/price-items/{id}",
		Summary:     "Aktualizuj ceníkovou položku",
		Tags:        []string{"price-items"},
	}, func(ctx context.Context, in *UpdateInput) (*GetOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var item models.PriceItem
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&item, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("položka %d nenalezena", in.ID), err)
		}
		upd := in.Body.toModel()
		item.Name = upd.Name
		item.CatalogNo = upd.CatalogNo
		item.EAN = upd.EAN
		item.UnitName = upd.UnitName
		item.UnitPriceHal = upd.UnitPriceHal
		item.VatRateBps = upd.VatRateBps
		item.TrackStock = upd.TrackStock
		item.AllowNegativeStock = upd.AllowNegativeStock
		if err := db.WithContext(ctx).Save(&item).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení selhalo", err)
		}
		out := PriceItemOut{PriceItem: item}
		if item.TrackStock {
			out.StockQuantity = stockQty(ctx, item.ID)
		}
		return &GetOutput{Body: out}, nil
	})

	// ── ARCHIVE (DELETE soft) ─────────────────────────────────────────────
	type ArchiveInput struct {
		ID uint `path:"id"`
	}

	huma.Register(api, huma.Operation{
		OperationID:   "archive-price-item",
		Method:        "DELETE",
		Path:          "/price-items/{id}",
		Summary:       "Archivuj ceníkovou položku",
		Tags:          []string{"price-items"},
		DefaultStatus: 204,
	}, func(ctx context.Context, in *ArchiveInput) (*struct{}, error) {
		userID := auth.UserIDFromCtx(ctx)
		var item models.PriceItem
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&item, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("položka %d nenalezena", in.ID), err)
		}
		item.Archived = true
		db.WithContext(ctx).Save(&item) //nolint:errcheck
		return nil, nil
	})

	// ── STOCK MOVEMENTS — LIST ────────────────────────────────────────────
	type MovListInput struct {
		ID uint `path:"id" doc:"ID ceníkové položky"`
	}
	type MovListOutput struct {
		Body []models.StockMovement
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-stock-movements",
		Method:      "GET",
		Path:        "/price-items/{id}/movements",
		Summary:     "Skladové pohyby položky",
		Tags:        []string{"price-items"},
	}, func(ctx context.Context, in *MovListInput) (*MovListOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		// Ověříme vlastnictví přes PriceItem
		var item models.PriceItem
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&item, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound("položka nenalezena", err)
		}
		var movs []models.StockMovement
		if err := db.WithContext(ctx).Where("price_item_id = ?", in.ID).
			Order("created_at desc").Find(&movs).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		return &MovListOutput{Body: movs}, nil
	})

	// ── STOCK MOVEMENTS — CREATE (ruční pohyb) ────────────────────────────
	type MovCreateInput struct {
		ID   uint               `path:"id"`
		Body StockMovementInput
	}
	type MovCreateOutput struct {
		Body models.StockMovement
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-stock-movement",
		Method:        "POST",
		Path:          "/price-items/{id}/movements",
		Summary:       "Přidej ruční skladový pohyb",
		Tags:          []string{"price-items"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *MovCreateInput) (*MovCreateOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var item models.PriceItem
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&item, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound("položka nenalezena", err)
		}

		qty, err := strconv.ParseFloat(in.Body.Quantity, 64)
		if err != nil || in.Body.Quantity == "" {
			return nil, huma.Error422UnprocessableEntity("neplatné množství", nil)
		}

		// Kontrola záporného stavu pokud nastaveno
		if !item.AllowNegativeStock && qty < 0 {
			current := stockQty(ctx, item.ID)
			if current+qty < 0 {
				return nil, huma.Error422UnprocessableEntity(
					fmt.Sprintf("pohyb by způsobil záporný stav skladu (aktuálně %.4g ks)", current), nil)
			}
		}

		mov := models.StockMovement{
			PriceItemID: item.ID,
			Quantity:    in.Body.Quantity,
			Note:        in.Body.Note,
		}
		if err := db.WithContext(ctx).Create(&mov).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("chyba uložení pohybu", err)
		}
		return &MovCreateOutput{Body: mov}, nil
	})
}
