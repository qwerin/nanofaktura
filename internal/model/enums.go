package model

// Membership roles.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

// Account VAT modes.
const (
	VatModeNonPayer         = "non_vat_payer"
	VatModePayer            = "vat_payer"
	VatModeIdentifiedPerson = "identified_person"
)

// Document types (invoices and number formats).
const (
	DocInvoice    = "invoice"
	DocProforma   = "proforma"
	DocCorrection = "correction"
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
