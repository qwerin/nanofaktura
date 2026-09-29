package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
)

// PriceItem is a price list entry (SPEC §7.1).
type PriceItem struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	SKU              string     `json:"sku"`
	UnitName         string     `json:"unit_name"`
	UnitPrice        int64      `json:"unit_price" doc:"Minor units; VAT included when prices_include_vat"`
	VatRateBps       int32      `json:"vat_rate_bps"`
	PricesIncludeVat bool       `json:"prices_include_vat"`
	Currency         string     `json:"currency"`
	TrackStock       bool       `json:"track_stock"`
	StockQuantity    string     `json:"stock_quantity" example:"12.5" doc:"Σ stock moves (meaningful with track_stock)"`
	MinStock         string     `json:"min_stock" doc:"Low stock threshold; empty = none"`
	LowStock         bool       `json:"low_stock" doc:"track_stock and stock_quantity ≤ min_stock"`
	Archived         bool       `json:"archived"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	Note             string     `json:"note"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func toPriceItem(m *model.PriceItem) PriceItem {
	out := PriceItem{
		ID: m.ID, Name: m.Name, SKU: m.SKU, UnitName: m.UnitName, UnitPrice: m.UnitPrice,
		VatRateBps: m.VatRateBps, PricesIncludeVat: m.PricesIncludeVat, Currency: m.Currency,
		TrackStock: m.TrackStock, StockQuantity: billing.FormatQuantity(m.StockQuantityMilli),
		Archived: m.ArchivedAt != nil, ArchivedAt: m.ArchivedAt, Note: m.Note,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.MinStockMilli != nil {
		out.MinStock = billing.FormatQuantity(*m.MinStockMilli)
		out.LowStock = m.TrackStock && m.StockQuantityMilli <= *m.MinStockMilli
	}
	return out
}

type PriceItemCreate struct {
	Name             string `json:"name" minLength:"1" maxLength:"500"`
	SKU              string `json:"sku,omitempty" maxLength:"100"`
	UnitName         string `json:"unit_name,omitempty" maxLength:"20"`
	UnitPrice        int64  `json:"unit_price,omitempty" minimum:"-100000000000000" maximum:"100000000000000"`
	VatRateBps       *int32 `json:"vat_rate_bps,omitempty" minimum:"0" maximum:"10000" doc:"Default: account default_vat_rate_bps"`
	PricesIncludeVat bool   `json:"prices_include_vat,omitempty"`
	Currency         string `json:"currency,omitempty" pattern:"^[A-Z]{3}$" doc:"Default: account default_currency"`
	TrackStock       bool   `json:"track_stock,omitempty"`
	StockQuantity    string `json:"stock_quantity,omitempty" maxLength:"20" doc:"Initial stock (track_stock only); recorded as a manual stock move"`
	MinStock         string `json:"min_stock,omitempty" maxLength:"20" doc:"Low stock threshold; empty = none"`
	Note             string `json:"note,omitempty" maxLength:"5000"`
}

// PriceItemPatch: nil = unchanged. stock_quantity is changed only by stock moves.
type PriceItemPatch struct {
	Name             *string `json:"name,omitempty" minLength:"1" maxLength:"500"`
	SKU              *string `json:"sku,omitempty" maxLength:"100"`
	UnitName         *string `json:"unit_name,omitempty" maxLength:"20"`
	UnitPrice        *int64  `json:"unit_price,omitempty" minimum:"-100000000000000" maximum:"100000000000000"`
	VatRateBps       *int32  `json:"vat_rate_bps,omitempty" minimum:"0" maximum:"10000"`
	PricesIncludeVat *bool   `json:"prices_include_vat,omitempty"`
	Currency         *string `json:"currency,omitempty" pattern:"^[A-Z]{3}$"`
	TrackStock       *bool   `json:"track_stock,omitempty"`
	MinStock         *string `json:"min_stock,omitempty" maxLength:"20" doc:"Empty string clears the threshold"`
	Archived         *bool   `json:"archived,omitempty" doc:"true archives (hidden from the default list), false restores"`
	Note             *string `json:"note,omitempty" maxLength:"5000"`
}

func (s *server) registerPriceItems(g huma.API) {
	huma.Get(g, "/price-items", s.listPriceItems)
	huma.Post(g, "/price-items", s.createPriceItem, status(http.StatusCreated), auth.ForEditors)
	huma.Get(g, "/price-items/{id}", s.getPriceItem)
	huma.Patch(g, "/price-items/{id}", s.patchPriceItem, auth.ForEditors)
	huma.Delete(g, "/price-items/{id}", s.deletePriceItem, status(http.StatusNoContent), auth.ForEditors)
}

type priceItemID struct {
	ID uint `path:"id"`
}

// likePattern is a case-insensitive LIKE pattern (use with ESCAPE '\').
func likePattern(q string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(q)) + "%"
}

func (s *server) listPriceItems(ctx context.Context, in *struct {
	PageParams
	Query    string `query:"query" doc:"Name or SKU (case-insensitive substring)"`
	Archived bool   `query:"archived" doc:"true = only archived items, false (default) = only active"`
	LowStock bool   `query:"low_stock" doc:"Only tracked items at or below min_stock"`
}) (*Out[ListResponse[PriceItem]], error) {
	q := s.scoped(ctx).Model(&model.PriceItem{})
	if in.Archived {
		q = q.Where("archived_at IS NOT NULL")
	} else {
		q = q.Where("archived_at IS NULL")
	}
	if in.LowStock {
		q = q.Where("track_stock = ? AND min_stock_milli IS NOT NULL AND stock_quantity_milli <= min_stock_milli", true)
	}
	if qs := strings.TrimSpace(in.Query); qs != "" {
		like := likePattern(qs)
		q = q.Where(`(LOWER(name) LIKE ? ESCAPE '\' OR LOWER(sku) LIKE ? ESCAPE '\')`, like, like)
	}
	return paginate(q.Order("LOWER(name), id"), in.PageParams, toPriceItem)
}

func (s *server) getPriceItem(ctx context.Context, in *priceItemID) (*Out[PriceItem], error) {
	m, err := loadPriceItem(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	return &Out[PriceItem]{Body: toPriceItem(m)}, nil
}

func loadPriceItem(ctx context.Context, db *gorm.DB, id uint) (*model.PriceItem, error) {
	var m model.PriceItem
	if err := db.Scopes(inAccount(ctx)).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "price item")
	}
	return &m, nil
}

// parseMinStock parses an optional threshold ("" = none).
func parseMinStock(v string) (*int64, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	q, err := billing.ParseQuantity(v)
	if err != nil {
		return nil, invalid("min_stock", err.Error())
	}
	return &q, nil
}

func (s *server) createPriceItem(ctx context.Context, in *struct{ Body PriceItemCreate }) (*Out[PriceItem], error) {
	acc := auth.AccountFrom(ctx)
	b := &in.Body
	m := &model.PriceItem{
		AccountID: acc.ID, SKU: strings.TrimSpace(b.SKU), UnitName: strings.TrimSpace(b.UnitName),
		UnitPrice: b.UnitPrice, VatRateBps: acc.DefaultVatRateBps, PricesIncludeVat: b.PricesIncludeVat,
		Currency: defaultStr(b.Currency, acc.DefaultCurrency), TrackStock: b.TrackStock, Note: b.Note,
	}
	if m.Name = strings.TrimSpace(b.Name); m.Name == "" {
		return nil, invalid("name", "name must not be empty")
	}
	apply(&m.VatRateBps, b.VatRateBps)
	var err error
	if m.MinStockMilli, err = parseMinStock(b.MinStock); err != nil {
		return nil, err
	}
	var initial int64
	if b.StockQuantity != "" {
		if !b.TrackStock {
			return nil, invalid("stock_quantity", "an initial stock requires track_stock")
		}
		if initial, err = billing.ParseQuantity(b.StockQuantity); err != nil {
			return nil, invalid("stock_quantity", err.Error())
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return dbErr(err, "price item")
		}
		if initial == 0 {
			return recordPriceItem(ctx, tx, events.PriceItemCreated, m)
		}
		mv := &model.StockMove{AccountID: acc.ID, PriceItemID: m.ID, Direction: model.StockIn,
			QuantityMilli: initial, MovedOn: s.today(), Note: "initial stock"}
		if initial < 0 {
			mv.Direction, mv.QuantityMilli = model.StockOut, -initial
		}
		if err := addStockMove(tx, mv); err != nil {
			return err
		}
		if err := tx.First(m, m.ID).Error; err != nil {
			return err
		}
		return recordPriceItem(ctx, tx, events.PriceItemCreated, m)
	})
	if err != nil {
		return nil, err
	}
	return &Out[PriceItem]{Body: toPriceItem(m)}, nil
}

func (s *server) patchPriceItem(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body PriceItemPatch
}) (*Out[PriceItem], error) {
	p := &in.Body
	m, err := loadPriceItem(ctx, s.db.WithContext(ctx), in.ID)
	if err != nil {
		return nil, err
	}
	if p.Name != nil {
		if m.Name = strings.TrimSpace(*p.Name); m.Name == "" {
			return nil, invalid("name", "name must not be empty")
		}
	}
	if p.SKU != nil {
		m.SKU = strings.TrimSpace(*p.SKU)
	}
	if p.UnitName != nil {
		m.UnitName = strings.TrimSpace(*p.UnitName)
	}
	apply(&m.UnitPrice, p.UnitPrice)
	apply(&m.VatRateBps, p.VatRateBps)
	apply(&m.PricesIncludeVat, p.PricesIncludeVat)
	apply(&m.Currency, p.Currency)
	apply(&m.TrackStock, p.TrackStock)
	apply(&m.Note, p.Note)
	if p.MinStock != nil {
		if m.MinStockMilli, err = parseMinStock(*p.MinStock); err != nil {
			return nil, err
		}
	}
	if p.Archived != nil {
		switch {
		case *p.Archived && m.ArchivedAt == nil:
			now := s.deps.Now()
			m.ArchivedAt = &now
		case !*p.Archived:
			m.ArchivedAt = nil
		}
	}
	// stock_quantity_milli is owned by stock moves; never overwrite it here
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Scopes(inAccount(ctx)).Omit("stock_quantity_milli").Save(m).Error; err != nil {
			return dbErr(err, "price item")
		}
		return recordPriceItem(ctx, tx, events.PriceItemUpdated, m)
	})
	if err != nil {
		return nil, err
	}
	return s.getPriceItem(ctx, &priceItemID{ID: m.ID})
}

// deletePriceItem deletes the item with its stock moves; document lines that
// referenced it keep their content and lose the link.
func (s *server) deletePriceItem(ctx context.Context, in *priceItemID) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := loadPriceItem(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if err := tx.Where("price_item_id = ?", m.ID).Delete(&model.StockMove{}).Error; err != nil {
			return dbErr(err, "stock move")
		}
		for _, t := range []any{&model.InvoiceLine{}, &model.ExpenseLine{}} {
			if err := tx.Model(t).Where("price_item_id = ?", m.ID).Update("price_item_id", nil).Error; err != nil {
				return dbErr(err, "price item")
			}
		}
		if err := tx.Delete(m).Error; err != nil {
			return dbErr(err, "price item")
		}
		return recordPriceItem(ctx, tx, events.PriceItemDeleted, m)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}
