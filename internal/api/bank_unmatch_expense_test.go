package api_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

// TestBankUnmatchExpense: unmatching an outgoing transaction auto-matched to
// an expense removes the expense payment, reopens the expense and records
// expense_payment.deleted + bank.unmatched.
func TestBankUnmatchExpense(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	bank := fioBank(a)
	supplier := "Jan Novák"
	exp := createExp(a, api.ExpenseCreate{VariableSymbol: strPtr("1234"), ExpenseSupplierFields: api.ExpenseSupplierFields{SupplierName: &supplier},
		Lines: []api.InvoiceLineInput{line("Nájem", "1", 250050, i32(0))}})
	importOK(a, bank.ID, fixtureFile(t, "fio_transactions.json"))

	tx3 := txByExternal(a, "26543210003")
	if tx3.MatchedExpenseID == nil || *tx3.MatchedExpenseID != exp.ID || getExp(a, exp.ID).Status != "paid" {
		t.Fatalf("precondition: %+v", tx3)
	}
	un := doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, tx3.ID, "/unmatch"), nil)
	if un.PaymentID != nil || un.MatchedExpenseID != nil || un.State == "matched" {
		t.Fatalf("unmatch: %+v", un)
	}
	got := getExp(a, exp.ID)
	if got.Status != "open" || got.PaidOn != "" || len(got.Payments) != 0 {
		t.Fatalf("expense after unmatch: %+v", got)
	}
	var n int64
	ts.db.Model(&model.ExpensePayment{}).Where("expense_id = ?", exp.ID).Count(&n)
	if n != 0 {
		t.Fatalf("expense payments left: %d", n)
	}
	for _, name := range []string{"expense_payment.deleted", "bank.unmatched"} {
		var c int64
		ts.db.Model(&model.Event{}).Where("name = ?", name).Count(&c)
		if c != 1 {
			t.Errorf("%s events: %d", name, c)
		}
	}
	r, body := a.do("POST", txURL(a, tx3.ID, "/unmatch"), nil)
	assertError(t, r, body, http.StatusConflict, "not matched")

	// matched again manually → paid again
	doJSON[api.BankTransaction](a, http.StatusOK, "POST", txURL(a, tx3.ID, "/match"), api.BankTransactionMatch{ExpenseID: &exp.ID})
	if got := getExp(a, exp.ID); got.Status != "paid" || len(got.Payments) != 1 {
		t.Fatalf("rematch: %+v", got)
	}
}
