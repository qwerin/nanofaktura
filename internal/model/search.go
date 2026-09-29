package model

import (
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/search"
)

// Searchable records keep search.Text(SearchSource()) in their SearchText
// column (global search, SPEC §7.15). The BeforeSave hooks refresh it on
// Create/Save; db.Migrate backfills rows written before the column existed.
// Updates of single columns (Update/UpdateColumn) must not touch the
// searchable fields.
type Searchable interface {
	SearchSource() []string
}

func (m *Invoice) SearchSource() []string {
	return []string{m.Number, m.VariableSymbol, m.ClientName, m.ClientFullName, m.ClientRegistrationNo, m.ClientVatNo, m.ClientEmail}
}

func (m *Expense) SearchSource() []string {
	return []string{m.Number, m.OriginalNumber, m.VariableSymbol, m.SupplierName, m.SupplierFullName,
		m.SupplierRegistrationNo, m.SupplierVatNo, m.Description}
}

func (m *Subject) SearchSource() []string {
	custom := ""
	if m.CustomID != nil {
		custom = *m.CustomID
	}
	return []string{m.Name, m.FullName, m.RegistrationNo, m.VatNo, m.Email, custom}
}

func (m *PriceItem) SearchSource() []string { return []string{m.Name, m.SKU} }

func (m *Invoice) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}

func (m *Expense) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}

func (m *Subject) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}

func (m *PriceItem) BeforeSave(*gorm.DB) error {
	m.SearchText = search.Text(m.SearchSource()...)
	return nil
}
