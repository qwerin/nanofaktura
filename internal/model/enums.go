package model

// Membership roles (SPEC §7.13). Permissions per role: see auth.Allow.
const (
	RoleOwner      = "owner"      // everything
	RoleAdmin      = "admin"      // everything except managing owners (and deleting the account)
	RoleAccountant = "accountant" // read-only + exports/reports
	RoleMember     = "member"     // documents, subjects, expenses; no settings
)

// Account VAT modes.
const (
	VatModeNonPayer         = "non_vat_payer"
	VatModePayer            = "vat_payer"
	VatModeIdentifiedPerson = "identified_person"
)

// VAT periods of a VAT payer (Account.VatPeriod; "" means month).
const (
	VatPeriodMonth   = "month"
	VatPeriodQuarter = "quarter"
)

// Document types (invoices and number formats).
const (
	DocInvoice    = "invoice"
	DocProforma   = "proforma"
	DocCorrection = "correction"
	// DocTaxDocument is the tax document for a received payment (daňový
	// doklad k přijaté platbě, § 28 ZDPH), issued automatically for every
	// payment of a VAT payer's proforma.
	DocTaxDocument = "tax_document"
)

// Supply types of a reverse-charge document to/from another EU member state
// (Invoice.SupplyType, Expense.SupplyType; "" = services).
const (
	SupplyServices = "services" // § 9 odst. 1 / § 24 — "daň odvede zákazník"
	SupplyGoods    = "goods"    // § 64 / § 25 — exempt intra-EU supply of goods
)

// Stored invoice statuses. "overdue" is derived on read, never stored.
const (
	StatusOpen          = "open"
	StatusSent          = "sent"
	StatusPaid          = "paid"
	StatusCancelled     = "cancelled"
	StatusUncollectible = "uncollectible"
)

// Subject types.
const (
	SubjectCustomer = "customer"
	SubjectSupplier = "supplier"
	SubjectBoth     = "both"
)
