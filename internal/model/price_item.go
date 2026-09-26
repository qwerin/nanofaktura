package model

import "time"

// PriceItem is a price list entry (SPEC §7.1). StockQuantityMilli is always
// equal to Σ of the item's StockMoves (in = +, out = −); it is only ever
// changed together with a stock move, in the same transaction.
type PriceItem struct {
	ID                 uint
	AccountID          uint   `gorm:"not null;index"`
	Name               string `gorm:"not null"`
	SKU                string `gorm:"index"`
	UnitName           string
	UnitPrice          int64
	VatRateBps         int32
	PricesIncludeVat   bool
	Currency           string `gorm:"not null"`
	TrackStock         bool
	StockQuantityMilli int64  `gorm:"not null;default:0"`
	MinStockMilli      *int64 // low stock threshold; nil = none
	ArchivedAt         *time.Time
	Note               string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Stock move directions.
const (
	StockIn  = "in"
	StockOut = "out"
)

// StockMove is one receipt/issue of a price item. Moves with InvoiceID or
// ExpenseID are generated from document lines and rewritten together with
// the document; the others are manual.
type StockMove struct {
	ID            uint
	AccountID     uint   `gorm:"not null;index"`
	PriceItemID   uint   `gorm:"not null;index"`
	Direction     string `gorm:"not null"`
	QuantityMilli int64  `gorm:"not null"` // always > 0; the sign is Direction
	MovedOn       string `gorm:"not null;index"`
	Note          string
	InvoiceID     *uint `gorm:"index"`
	ExpenseID     *uint `gorm:"index"`
	CreatedAt     time.Time
}

// Delta is the signed change of the stock quantity caused by the move.
func (m *StockMove) Delta() int64 {
	if m.Direction == StockOut {
		return -m.QuantityMilli
	}
	return m.QuantityMilli
}
