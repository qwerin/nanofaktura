package isdoc

import "encoding/xml"

// XML structure of isdoc-invoice-6.0.2.xsd. Field order is element order;
// optional elements use omitempty / pointers, required ones never do.

type Invoice struct {
	XMLName       xml.Name `xml:"Invoice"`
	Xmlns         string   `xml:"xmlns,attr"`
	Version       string   `xml:"version,attr"`
	DocumentType  int      `xml:"DocumentType"`
	ID            string   `xml:"ID"`
	UUID          string   `xml:"UUID"`
	IssuingSystem string   `xml:"IssuingSystem,omitempty"`
	IssueDate     string   `xml:"IssueDate"`
	TaxPointDate  string   `xml:"TaxPointDate,omitempty"`
	VATApplicable bool     `xml:"VATApplicable"`
	// Required, empty: no separate agreement on electronic invoicing.
	ElectronicPossibilityAgreementReference string `xml:"ElectronicPossibilityAgreementReference"`
	Note                                    *Note  `xml:"Note,omitempty"`
	LocalCurrency                           string `xml:"LocalCurrencyCode"`
	ForeignCurrency                         string `xml:"ForeignCurrencyCode,omitempty"`
	CurrRate                                string `xml:"CurrRate"`
	RefCurrRate                             string `xml:"RefCurrRate"`

	Supplier                   party                       `xml:"AccountingSupplierParty"`
	Customer                   party                       `xml:"AccountingCustomerParty"`
	OrderReferences            *orderReferences            `xml:"OrderReferences,omitempty"`
	OriginalDocumentReferences *originalDocumentReferences `xml:"OriginalDocumentReferences,omitempty"`
	Lines                      invoiceLines                `xml:"InvoiceLines"`
	NonTaxedDeposits           *nonTaxedDeposits           `xml:"NonTaxedDeposits,omitempty"`
	TaxedDeposits              *taxedDeposits              `xml:"TaxedDeposits,omitempty"`
	TaxTotal                   TaxTotal                    `xml:"TaxTotal"`
	Totals                     LegalMonetaryTotal          `xml:"LegalMonetaryTotal"`
	PaymentMeans               *PaymentMeans               `xml:"PaymentMeans,omitempty"`
}

type Note struct {
	Text string `xml:",chardata"`
}

type party struct {
	Party Party `xml:"Party"`
}

type Party struct {
	ID           string                  `xml:"PartyIdentification>ID"`
	Name         string                  `xml:"PartyName>Name"`
	Address      PostalAddress           `xml:"PostalAddress"`
	TaxSchemes   []PartyTaxScheme        `xml:"PartyTaxScheme,omitempty"`
	Registration *RegisterIdentification `xml:"RegisterIdentification,omitempty"`
	Contact      *Contact                `xml:"Contact,omitempty"`
}

type PostalAddress struct {
	StreetName     string  `xml:"StreetName"`
	BuildingNumber string  `xml:"BuildingNumber"`
	CityName       string  `xml:"CityName"`
	PostalZone     string  `xml:"PostalZone"`
	Country        Country `xml:"Country"`
}

type Country struct {
	Code string `xml:"IdentificationCode"`
	Name string `xml:"Name"`
}

type PartyTaxScheme struct {
	CompanyID string `xml:"CompanyID"`
	TaxScheme string `xml:"TaxScheme"`
}

type RegisterIdentification struct {
	Preformatted string `xml:"Preformatted"`
}

type Contact struct {
	Name           string `xml:"Name,omitempty"`
	Telephone      string `xml:"Telephone,omitempty"`
	ElectronicMail string `xml:"ElectronicMail,omitempty"`
}

type orderReferences struct {
	Refs []OrderReference `xml:"OrderReference"`
}

type OrderReference struct {
	IDAttr          string `xml:"id,attr,omitempty"`
	SalesOrderID    string `xml:"SalesOrderID"`
	ExternalOrderID string `xml:"ExternalOrderID,omitempty"`
}

type originalDocumentReferences struct {
	Refs []OriginalDocumentReference `xml:"OriginalDocumentReference"`
}

type OriginalDocumentReference struct {
	IDAttr    string `xml:"id,attr,omitempty"`
	ID        string `xml:"ID"`
	IssueDate string `xml:"IssueDate,omitempty"`
	UUID      string `xml:"UUID,omitempty"`
}

type invoiceLines struct {
	Lines []InvoiceLine `xml:"InvoiceLine"`
}

type InvoiceLine struct {
	ID                                  string                `xml:"ID"`
	Quantity                            *Quantity             `xml:"InvoicedQuantity,omitempty"`
	LineExtensionAmountCurr             string                `xml:"LineExtensionAmountCurr,omitempty"`
	LineExtensionAmount                 string                `xml:"LineExtensionAmount"`
	LineExtensionAmountTaxInclusiveCurr string                `xml:"LineExtensionAmountTaxInclusiveCurr,omitempty"`
	LineExtensionAmountTaxInclusive     string                `xml:"LineExtensionAmountTaxInclusive"`
	LineExtensionTaxAmount              string                `xml:"LineExtensionTaxAmount"`
	UnitPrice                           string                `xml:"UnitPrice"`
	UnitPriceTaxInclusive               string                `xml:"UnitPriceTaxInclusive"`
	TaxCategory                         ClassifiedTaxCategory `xml:"ClassifiedTaxCategory"`
	VATNote                             *Note                 `xml:"VATNote,omitempty"`
	Item                                *Item                 `xml:"Item,omitempty"`
}

type Quantity struct {
	UnitCode string `xml:"unitCode,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type ClassifiedTaxCategory struct {
	Percent              string `xml:"Percent"`
	VATCalculationMethod int    `xml:"VATCalculationMethod"`
	VATApplicable        bool   `xml:"VATApplicable"`
}

type Item struct {
	Description string `xml:"Description,omitempty"`
}

type nonTaxedDeposits struct {
	Deposits []NonTaxedDeposit `xml:"NonTaxedDeposit"`
}

type taxedDeposits struct {
	Deposits []TaxedDeposit `xml:"TaxedDeposit"`
}

// TaxedDeposit is an advance already taxed by a tax document for a received
// payment (one per document and rate), deducted on the final invoice.
type TaxedDeposit struct {
	ID                            string                `xml:"ID"`
	VariableSymbol                string                `xml:"VariableSymbol"`
	TaxableDepositAmountCurr      string                `xml:"TaxableDepositAmountCurr,omitempty"`
	TaxableDepositAmount          string                `xml:"TaxableDepositAmount"`
	TaxInclusiveDepositAmountCurr string                `xml:"TaxInclusiveDepositAmountCurr,omitempty"`
	TaxInclusiveDepositAmount     string                `xml:"TaxInclusiveDepositAmount"`
	TaxCategory                   ClassifiedTaxCategory `xml:"ClassifiedTaxCategory"`
}

type NonTaxedDeposit struct {
	ID                string `xml:"ID"`
	VariableSymbol    string `xml:"VariableSymbol"`
	DepositAmountCurr string `xml:"DepositAmountCurr,omitempty"`
	DepositAmount     string `xml:"DepositAmount"`
}

type TaxTotal struct {
	SubTotals     []TaxSubTotal `xml:"TaxSubTotal"`
	TaxAmountCurr string        `xml:"TaxAmountCurr,omitempty"`
	TaxAmount     string        `xml:"TaxAmount"`
}

type TaxSubTotal struct {
	TaxableAmountCurr                string      `xml:"TaxableAmountCurr,omitempty"`
	TaxableAmount                    string      `xml:"TaxableAmount"`
	TaxAmountCurr                    string      `xml:"TaxAmountCurr,omitempty"`
	TaxAmount                        string      `xml:"TaxAmount"`
	TaxInclusiveAmountCurr           string      `xml:"TaxInclusiveAmountCurr,omitempty"`
	TaxInclusiveAmount               string      `xml:"TaxInclusiveAmount"`
	AlreadyClaimedTaxableAmountCurr  string      `xml:"AlreadyClaimedTaxableAmountCurr,omitempty"`
	AlreadyClaimedTaxableAmount      string      `xml:"AlreadyClaimedTaxableAmount"`
	AlreadyClaimedTaxAmountCurr      string      `xml:"AlreadyClaimedTaxAmountCurr,omitempty"`
	AlreadyClaimedTaxAmount          string      `xml:"AlreadyClaimedTaxAmount"`
	AlreadyClaimedTaxInclusiveCurr   string      `xml:"AlreadyClaimedTaxInclusiveAmountCurr,omitempty"`
	AlreadyClaimedTaxInclusiveAmount string      `xml:"AlreadyClaimedTaxInclusiveAmount"`
	DifferenceTaxableAmountCurr      string      `xml:"DifferenceTaxableAmountCurr,omitempty"`
	DifferenceTaxableAmount          string      `xml:"DifferenceTaxableAmount"`
	DifferenceTaxAmountCurr          string      `xml:"DifferenceTaxAmountCurr,omitempty"`
	DifferenceTaxAmount              string      `xml:"DifferenceTaxAmount"`
	DifferenceTaxInclusiveCurr       string      `xml:"DifferenceTaxInclusiveAmountCurr,omitempty"`
	DifferenceTaxInclusiveAmount     string      `xml:"DifferenceTaxInclusiveAmount"`
	TaxCategory                      TaxCategory `xml:"TaxCategory"`
}

type TaxCategory struct {
	Percent       string `xml:"Percent"`
	VATApplicable bool   `xml:"VATApplicable"`
}

type LegalMonetaryTotal struct {
	TaxExclusiveAmount                   string `xml:"TaxExclusiveAmount"`
	TaxExclusiveAmountCurr               string `xml:"TaxExclusiveAmountCurr,omitempty"`
	TaxInclusiveAmount                   string `xml:"TaxInclusiveAmount"`
	TaxInclusiveAmountCurr               string `xml:"TaxInclusiveAmountCurr,omitempty"`
	AlreadyClaimedTaxExclusiveAmount     string `xml:"AlreadyClaimedTaxExclusiveAmount"`
	AlreadyClaimedTaxExclusiveAmountCurr string `xml:"AlreadyClaimedTaxExclusiveAmountCurr,omitempty"`
	AlreadyClaimedTaxInclusiveAmount     string `xml:"AlreadyClaimedTaxInclusiveAmount"`
	AlreadyClaimedTaxInclusiveAmountCurr string `xml:"AlreadyClaimedTaxInclusiveAmountCurr,omitempty"`
	DifferenceTaxExclusiveAmount         string `xml:"DifferenceTaxExclusiveAmount"`
	DifferenceTaxExclusiveAmountCurr     string `xml:"DifferenceTaxExclusiveAmountCurr,omitempty"`
	DifferenceTaxInclusiveAmount         string `xml:"DifferenceTaxInclusiveAmount"`
	DifferenceTaxInclusiveAmountCurr     string `xml:"DifferenceTaxInclusiveAmountCurr,omitempty"`
	PayableRoundingAmount                string `xml:"PayableRoundingAmount,omitempty"`
	PayableRoundingAmountCurr            string `xml:"PayableRoundingAmountCurr,omitempty"`
	PaidDepositsAmount                   string `xml:"PaidDepositsAmount"`
	PaidDepositsAmountCurr               string `xml:"PaidDepositsAmountCurr,omitempty"`
	PayableAmount                        string `xml:"PayableAmount"`
	PayableAmountCurr                    string `xml:"PayableAmountCurr,omitempty"`
}

type PaymentMeans struct {
	Payments []Payment `xml:"Payment"`
}

type Payment struct {
	PaidAmount string          `xml:"PaidAmount"`
	Code       int             `xml:"PaymentMeansCode"`
	Details    *PaymentDetails `xml:"Details,omitempty"`
}

type PaymentDetails struct {
	PaymentDueDate string `xml:"PaymentDueDate"`
	ID             string `xml:"ID"`
	BankCode       string `xml:"BankCode"`
	Name           string `xml:"Name"`
	IBAN           string `xml:"IBAN"`
	BIC            string `xml:"BIC"`
	VariableSymbol string `xml:"VariableSymbol,omitempty"`
}
