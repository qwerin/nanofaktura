// Package billing is the pure domain logic of documents (invoices, later
// expenses): quantity parsing, integer rounding, totals and VAT recapitulation,
// status derivation and the invoice state machine. No DB, no HTTP, no floats.
//
// Money is int64 in minor units, quantities are int64 thousandths ("milli"),
// VAT rates are basis points (2100 = 21 %), dates are "YYYY-MM-DD" strings.
package billing
