package models

// PriceItem je položka ceníku / skladu.
// Pokud TrackStock = true, eviduje se stav pomocí StockMovement záznamů.
// Aktuální stav skladu = SUM(quantity) přes StockMovement pro daný PriceItemID.
type PriceItem struct {
	Base

	// Vlastník — 0 = single-user, >0 = user ID (multi-user mód)
	UserID uint `gorm:"not null;default:0;index;uniqueIndex:uidx_price_catalog_user;uniqueIndex:uidx_price_ean_user" json:"user_id,omitempty"`

	Name      string  `gorm:"not null"                                                                            json:"name"`
	CatalogNo *string `gorm:"size:100;uniqueIndex:uidx_price_catalog_user,where:catalog_no IS NOT NULL"          json:"catalog_no,omitempty"`
	EAN       *string `gorm:"size:20;uniqueIndex:uidx_price_ean_user,where:ean IS NOT NULL"                       json:"ean,omitempty"`

	UnitName     string `gorm:"size:20;not null;default:'ks'" json:"unit_name"`
	UnitPriceHal int64  `gorm:"not null;default:0"            json:"unit_price_hal"`
	VatRateBps   int32  `gorm:"not null;default:0"            json:"vat_rate_bps"`

	// Sklad
	TrackStock         bool `gorm:"not null;default:false" json:"track_stock"`
	AllowNegativeStock bool `gorm:"not null;default:false" json:"allow_negative_stock"`

	Archived bool `gorm:"not null;default:false" json:"archived"`
}

// StockMovement je jeden skladový pohyb (příjem nebo výdej).
// Quantity je decimal string, kladný = příjem, záporný = výdej.
// Aktuální stav = SUM přes všechny pohyby dané položky.
type StockMovement struct {
	Base

	PriceItemID uint   `gorm:"not null;index"  json:"price_item_id"`
	Quantity    string `gorm:"not null"        json:"quantity"` // decimal, např. "10" nebo "-2.5"
	Note        string `json:"note,omitempty"`
	InvoiceID   *uint  `gorm:"index"           json:"invoice_id,omitempty"` // odkaz na fakturu, pokud byl pohyb z faktury
}
