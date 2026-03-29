package handler_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/models"
)

const minimalPriceItem = `{
	"name": "Testovací produkt",
	"unit_name": "ks",
	"unit_price_hal": 50000,
	"vat_rate_bps": 2100,
	"track_stock": false,
	"allow_negative_stock": false
}`

const stockPriceItem = `{
	"name": "Skladová položka",
	"catalog_no": "KAT-001",
	"unit_name": "ks",
	"unit_price_hal": 100000,
	"vat_rate_bps": 2100,
	"track_stock": true,
	"allow_negative_stock": false
}`

func TestPriceItem_Create(t *testing.T) {
	api := newTestAPIChi(t)
	w := do(t, api, "POST", "/price-items", minimalPriceItem)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var item models.PriceItem
	decodeBody(t, w, &item)
	if item.Name != "Testovací produkt" {
		t.Errorf("name = %q, want 'Testovací produkt'", item.Name)
	}
	if item.UnitPriceHal != 50000 {
		t.Errorf("unit_price_hal = %d, want 50000", item.UnitPriceHal)
	}
}

func TestPriceItem_List(t *testing.T) {
	api := newTestAPIChi(t)
	do(t, api, "POST", "/price-items", minimalPriceItem)
	do(t, api, "POST", "/price-items", `{"name": "Druhý produkt", "unit_name": "m", "unit_price_hal": 200000, "vat_rate_bps": 0, "track_stock": false, "allow_negative_stock": false}`)

	w := do(t, api, "GET", "/price-items", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var items []map[string]interface{}
	decodeBody(t, w, &items)
	if len(items) != 2 {
		t.Errorf("len = %d, want 2", len(items))
	}
}

func TestPriceItem_Get(t *testing.T) {
	api := newTestAPIChi(t)
	wc := do(t, api, "POST", "/price-items", minimalPriceItem)
	var created models.PriceItem
	decodeBody(t, wc, &created)

	w := do(t, api, "GET", fmt.Sprintf("/price-items/%d", created.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	var item models.PriceItem
	decodeBody(t, w, &item)
	if item.ID != created.ID {
		t.Errorf("id = %d, want %d", item.ID, created.ID)
	}
}

func TestPriceItem_Update(t *testing.T) {
	api := newTestAPIChi(t)
	wc := do(t, api, "POST", "/price-items", minimalPriceItem)
	var created models.PriceItem
	decodeBody(t, wc, &created)

	wu := do(t, api, "PUT", fmt.Sprintf("/price-items/%d", created.ID),
		`{"name": "Upravený produkt", "unit_name": "ks", "unit_price_hal": 75000, "vat_rate_bps": 2100, "track_stock": false, "allow_negative_stock": false}`)
	if wu.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", wu.Code, wu.Body.String())
	}
	var updated models.PriceItem
	decodeBody(t, wu, &updated)
	if updated.Name != "Upravený produkt" {
		t.Errorf("name = %q, want 'Upravený produkt'", updated.Name)
	}
	if updated.UnitPriceHal != 75000 {
		t.Errorf("unit_price_hal = %d, want 75000", updated.UnitPriceHal)
	}
}

func TestPriceItem_Archive(t *testing.T) {
	api := newTestAPIChi(t)
	wc := do(t, api, "POST", "/price-items", minimalPriceItem)
	var created models.PriceItem
	decodeBody(t, wc, &created)

	wa := do(t, api, "DELETE", fmt.Sprintf("/price-items/%d", created.ID), "")
	if wa.Code != http.StatusNoContent {
		t.Fatalf("archive status = %d; body: %s", wa.Code, wa.Body.String())
	}

	// Po archivaci by neměla být v defaultním výpisu
	wl := do(t, api, "GET", "/price-items", "")
	var items []map[string]interface{}
	decodeBody(t, wl, &items)
	if len(items) != 0 {
		t.Errorf("po archivaci len = %d, want 0", len(items))
	}

	// S ?archived=true by měla být viditelná
	wla := do(t, api, "GET", "/price-items?archived=true", "")
	var itemsArchived []map[string]interface{}
	decodeBody(t, wla, &itemsArchived)
	if len(itemsArchived) != 1 {
		t.Errorf("archived len = %d, want 1", len(itemsArchived))
	}
}

func TestPriceItem_StockMovement_Create(t *testing.T) {
	api := newTestAPIChi(t)
	wc := do(t, api, "POST", "/price-items", stockPriceItem)
	if wc.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", wc.Code, wc.Body.String())
	}
	var item models.PriceItem
	decodeBody(t, wc, &item)

	// Příjem 10 ks
	wm := do(t, api, "POST", fmt.Sprintf("/price-items/%d/movements", item.ID),
		`{"quantity": "10", "note": "Naskladnění"}`)
	if wm.Code != http.StatusCreated {
		t.Fatalf("movement status = %d; body: %s", wm.Code, wm.Body.String())
	}
	var mov models.StockMovement
	decodeBody(t, wm, &mov)
	if mov.Quantity != "10" {
		t.Errorf("quantity = %q, want '10'", mov.Quantity)
	}

	// Stav skladu by měl být 10
	wg := do(t, api, "GET", fmt.Sprintf("/price-items/%d", item.ID), "")
	var updated map[string]interface{}
	decodeBody(t, wg, &updated)
	if qty, ok := updated["stock_quantity"].(float64); !ok || qty != 10 {
		t.Errorf("stock_quantity = %v, want 10", updated["stock_quantity"])
	}
}

func TestPriceItem_StockMovement_NegativeBlocked(t *testing.T) {
	api := newTestAPIChi(t)
	wc := do(t, api, "POST", "/price-items", stockPriceItem) // allow_negative_stock: false
	var item models.PriceItem
	decodeBody(t, wc, &item)

	// Pokus o výdej -5 při prázdném skladu → 422
	wm := do(t, api, "POST", fmt.Sprintf("/price-items/%d/movements", item.ID),
		`{"quantity": "-5", "note": "Výdej při prázdném skladu"}`)
	if wm.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d; body: %s", wm.Code, wm.Body.String())
	}
}

func TestPriceItem_InvoiceAutoMovement(t *testing.T) {
	api := newTestAPIChi(t)

	// Vytvoříme ceníkovou položku se sledováním skladu
	wc := do(t, api, "POST", "/price-items", `{
		"name": "Fakturovaný produkt",
		"unit_name": "ks",
		"unit_price_hal": 200000,
		"vat_rate_bps": 2100,
		"track_stock": true,
		"allow_negative_stock": true
	}`)
	var item models.PriceItem
	decodeBody(t, wc, &item)

	// Ručně naskladníme 5 ks
	do(t, api, "POST", fmt.Sprintf("/price-items/%d/movements", item.ID), `{"quantity": "5"}`)

	// Vytvoříme fakturu s řádkem odkazujícím na ceníkovou položku
	invBody := fmt.Sprintf(`{
		"number": "F-STOCK-001",
		"issued_on": "2026-01-01",
		"taxable_fulfillment_due": "2026-01-01",
		"client_name": "Zákazník s.r.o.",
		"lines": [{
			"price_item_id": %d,
			"name": "Fakturovaný produkt",
			"quantity": "2",
			"unit_name": "ks",
			"unit_price_hal": 200000,
			"vat_rate_bps": 2100
		}]
	}`, item.ID)

	wi := do(t, api, "POST", "/invoices", invBody)
	if wi.Code != http.StatusCreated {
		t.Fatalf("invoice status = %d; body: %s", wi.Code, wi.Body.String())
	}

	// Stav skladu by měl být 5 - 2 = 3
	wg := do(t, api, "GET", fmt.Sprintf("/price-items/%d", item.ID), "")
	var updated map[string]interface{}
	decodeBody(t, wg, &updated)
	if qty, ok := updated["stock_quantity"].(float64); !ok || qty != 3 {
		t.Errorf("stock_quantity po fakturaci = %v, want 3", updated["stock_quantity"])
	}
}
