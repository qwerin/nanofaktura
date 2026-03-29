// Package handler – DTO typy oddělují API kontrakt od GORM modelů.
// Huma v2 generuje JSON Schema z Go typů: non-pointer fields = required,
// pointer fields / omitempty = optional. GORM modely nesmíme používat
// přímo jako Huma input, protože by se stala required i computed pole.
package handler

import "github.com/qwerin/nanofaktura/internal/models"

// LineInput je klientský vstup pro jednu řádkovou položku faktury.
type LineInput struct {
	Position     int    `json:"position,omitempty"`
	PriceItemID  *uint  `json:"price_item_id,omitempty"` // volitelný odkaz na ceníkovou položku
	Name         string `json:"name"`
	Quantity     string `json:"quantity,omitempty"`
	UnitName     string `json:"unit_name,omitempty"`
	UnitPriceHal int64  `json:"unit_price_hal"`
	VatRateBps   int32  `json:"vat_rate_bps,omitempty"`
}

// PriceItemInput je klientský vstup pro vytvoření / aktualizaci ceníkové položky.
type PriceItemInput struct {
	Name      string  `json:"name"`
	CatalogNo *string `json:"catalog_no,omitempty"`
	EAN       *string `json:"ean,omitempty"`

	UnitName     string `json:"unit_name,omitempty"`
	UnitPriceHal int64  `json:"unit_price_hal"`
	VatRateBps   int32  `json:"vat_rate_bps,omitempty"`

	TrackStock         bool `json:"track_stock,omitempty"`
	AllowNegativeStock bool `json:"allow_negative_stock,omitempty"`
}

func (in *PriceItemInput) toModel() models.PriceItem {
	return models.PriceItem{
		Name:               in.Name,
		CatalogNo:          in.CatalogNo,
		EAN:                in.EAN,
		UnitName:           orDefault(in.UnitName, "ks"),
		UnitPriceHal:       in.UnitPriceHal,
		VatRateBps:         in.VatRateBps,
		TrackStock:         in.TrackStock,
		AllowNegativeStock: in.AllowNegativeStock,
	}
}

// StockMovementInput je klientský vstup pro ruční pohyb skladu.
type StockMovementInput struct {
	Quantity string `json:"quantity"` // decimal, + příjem / - výdej
	Note     string `json:"note,omitempty"`
}

// InvoiceInput je klientský vstup pro vytvoření / aktualizaci faktury.
// Obsahuje pouze pole, která klient může nastavit — computed/readonly pole chybí záměrně.
type InvoiceInput struct {
	// Povinná pole
	Number                string      `json:"number"`
	IssuedOn              string      `json:"issued_on"`
	TaxableFulfillmentDue string      `json:"taxable_fulfillment_due"`
	ClientName            string      `json:"client_name"`
	Lines                 []LineInput `json:"lines"`

	// Volitelná pole (omitempty → Huma je neoznačí jako required)
	DocumentType string `json:"document_type,omitempty"`
	Language     string `json:"language,omitempty"`

	VariableSymbol string `json:"variable_symbol,omitempty"`
	OrderNumber    string `json:"order_number,omitempty"`
	CustomID       string `json:"custom_id,omitempty"`

	Due       int  `json:"due,omitempty"`
	SubjectID uint `json:"subject_id,omitempty"`

	YourName           string `json:"your_name,omitempty"`
	YourStreet         string `json:"your_street,omitempty"`
	YourCity           string `json:"your_city,omitempty"`
	YourZip            string `json:"your_zip,omitempty"`
	YourCountry        string `json:"your_country,omitempty"`
	YourRegistrationNo string `json:"your_registration_no,omitempty"`
	YourVatNo          string `json:"your_vat_no,omitempty"`

	ClientStreet         string `json:"client_street,omitempty"`
	ClientCity           string `json:"client_city,omitempty"`
	ClientZip            string `json:"client_zip,omitempty"`
	ClientCountry        string `json:"client_country,omitempty"`
	ClientRegistrationNo string `json:"client_registration_no,omitempty"`
	ClientVatNo          string `json:"client_vat_no,omitempty"`

	ClientHasDeliveryAddress bool   `json:"client_has_delivery_address,omitempty"`
	ClientDeliveryName       string `json:"client_delivery_name,omitempty"`
	ClientDeliveryStreet     string `json:"client_delivery_street,omitempty"`
	ClientDeliveryCity       string `json:"client_delivery_city,omitempty"`
	ClientDeliveryZip        string `json:"client_delivery_zip,omitempty"`
	ClientDeliveryCountry    string `json:"client_delivery_country,omitempty"`

	PaymentMethod string `json:"payment_method,omitempty"`
	BankAccount   string `json:"bank_account,omitempty"`
	IBAN          string `json:"iban,omitempty"`
	SwiftBIC      string `json:"swift_bic,omitempty"`

	Currency     string `json:"currency,omitempty"`
	ExchangeRate string `json:"exchange_rate,omitempty"`

	VatExempt               bool   `json:"vat_exempt,omitempty"`
	TransferredTaxLiability bool   `json:"transferred_tax_liability,omitempty"`
	VatPriceMode            string `json:"vat_price_mode,omitempty"`

	Note        string `json:"note,omitempty"`
	FooterNote  string `json:"footer_note,omitempty"`
	PrivateNote string `json:"private_note,omitempty"`
	Tags        string `json:"tags,omitempty"`
}

// toModel mapuje InvoiceInput na models.Invoice připravený k uložení.
func (in *InvoiceInput) toModel() models.Invoice {
	inv := models.Invoice{
		Number:                in.Number,
		IssuedOn:              in.IssuedOn,
		TaxableFulfillmentDue: in.TaxableFulfillmentDue,
		ClientName:            in.ClientName,

		DocumentType:   models.DocumentType(orDefault(in.DocumentType, string(models.DocInvoice))),
		Status:         models.StatusOpen,
		Language:       models.Language(orDefault(in.Language, string(models.LangCS))),
		VariableSymbol: in.VariableSymbol,
		OrderNumber:    in.OrderNumber,
		CustomID:       in.CustomID,
		Due:            orDefaultInt(in.Due, 14),
		SubjectID:      in.SubjectID,

		YourName:           in.YourName,
		YourStreet:         in.YourStreet,
		YourCity:           in.YourCity,
		YourZip:            in.YourZip,
		YourCountry:        orDefault(in.YourCountry, "CZ"),
		YourRegistrationNo: in.YourRegistrationNo,
		YourVatNo:          in.YourVatNo,

		ClientStreet:         in.ClientStreet,
		ClientCity:           in.ClientCity,
		ClientZip:            in.ClientZip,
		ClientCountry:        orDefault(in.ClientCountry, "CZ"),
		ClientRegistrationNo: in.ClientRegistrationNo,
		ClientVatNo:          in.ClientVatNo,

		ClientHasDeliveryAddress: in.ClientHasDeliveryAddress,
		ClientDeliveryName:       in.ClientDeliveryName,
		ClientDeliveryStreet:     in.ClientDeliveryStreet,
		ClientDeliveryCity:       in.ClientDeliveryCity,
		ClientDeliveryZip:        in.ClientDeliveryZip,
		ClientDeliveryCountry:    in.ClientDeliveryCountry,

		PaymentMethod: models.PaymentMethod(orDefault(in.PaymentMethod, string(models.PaymentBank))),
		BankAccount:   in.BankAccount,
		IBAN:          in.IBAN,
		SwiftBIC:      in.SwiftBIC,

		Currency:     orDefault(in.Currency, "CZK"),
		ExchangeRate: orDefault(in.ExchangeRate, "1"),

		VatExempt:               in.VatExempt,
		TransferredTaxLiability: in.TransferredTaxLiability,
		VatPriceMode:            orDefault(in.VatPriceMode, "without_vat"),

		Note:        in.Note,
		FooterNote:  in.FooterNote,
		PrivateNote: in.PrivateNote,
		Tags:        in.Tags,
	}

	for i, l := range in.Lines {
		inv.Lines = append(inv.Lines, models.InvoiceLine{
			PriceItemID:  l.PriceItemID,
			Position:     orDefaultInt(l.Position, i+1),
			Name:         l.Name,
			Quantity:     orDefault(l.Quantity, "1"),
			UnitName:     orDefault(l.UnitName, "ks"),
			UnitPriceHal: l.UnitPriceHal,
			VatRateBps:   l.VatRateBps,
		})
	}
	return inv
}

// SubjectInput je klientský vstup pro vytvoření / aktualizaci kontaktu.
// Neobsahuje gorm.Model → Huma nevyžaduje ID, CreatedAt, DeletedAt.
type SubjectInput struct {
	Name string `json:"name"` // povinné

	CustomID string `json:"custom_id,omitempty"`
	Type     string `json:"type,omitempty"`

	Street     string `json:"street,omitempty"`
	City       string `json:"city,omitempty"`
	Zip        string `json:"zip,omitempty"`
	Country    string `json:"country,omitempty"`
	RegistrationNo string `json:"registration_no,omitempty"`
	VatNo      string `json:"vat_no,omitempty"`
	LocalVatNo string `json:"local_vat_no,omitempty"`

	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Website string `json:"website,omitempty"`

	BankAccount string `json:"bank_account,omitempty"`
	IBAN        string `json:"iban,omitempty"`

	Note string `json:"note,omitempty"`

	DefaultPaymentMethod string `json:"default_payment_method,omitempty"`
	DefaultDue           int    `json:"default_due,omitempty"`
}

func (in *SubjectInput) toModel() models.Subject {
	var customID *string
	if in.CustomID != "" {
		customID = &in.CustomID
	}
	return models.Subject{
		CustomID:             customID,
		Type:                 models.SubjectType(orDefault(in.Type, string(models.SubjectCustomer))),
		Name:                 in.Name,
		Street:               in.Street,
		City:                 in.City,
		Zip:                  in.Zip,
		Country:              orDefault(in.Country, "CZ"),
		RegistrationNo:       in.RegistrationNo,
		VatNo:                in.VatNo,
		LocalVatNo:           in.LocalVatNo,
		Email:                in.Email,
		Phone:                in.Phone,
		Website:              in.Website,
		BankAccount:          in.BankAccount,
		IBAN:                 in.IBAN,
		Note:                 in.Note,
		DefaultPaymentMethod: models.PaymentMethod(orDefault(in.DefaultPaymentMethod, string(models.PaymentBank))),
		DefaultDue:           orDefaultInt(in.DefaultDue, 14),
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func orDefaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
