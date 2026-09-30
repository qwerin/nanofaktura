package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ErrorModel is the problem+json body of every error: huma's RFC 9457 model
// plus a machine-readable Code (SPEC §2 "Chyby"). Clients branch on Code,
// never on the English Detail. Errors without a specific code get a generic
// one derived from the status (see statusCodes).
type ErrorModel struct {
	huma.ErrorModel
	Code string `json:"code,omitempty" example:"has_invoices" doc:"Machine-readable reason (stable, snake_case). Generic per status when nothing more specific applies: bad_request, unauthorized, forbidden, not_found, conflict, gone, too_large, unsupported_media_type, validation_failed, rate_limited, internal, upstream_unavailable, upstream_timeout."`
}

// Error codes of domain errors. Keep in sync with web/src/api/errors.ts.
const (
	CodeValidation               = "validation_failed"
	CodeAlreadyExists            = "already_exists"
	CodeEmailTaken               = "email_taken"
	CodeSignupDisabled           = "signup_disabled"
	CodeLocked                   = "locked"
	CodeHasPayments              = "has_payments"
	CodeHasInvoices              = "has_invoices"
	CodeNotEditable              = "not_editable"
	CodeInvalidTransition        = "invalid_transition"
	CodeNoNumberFormat           = "no_number_format"
	CodeReferenced               = "referenced"
	CodeUsedByTemplate           = "used_by_template"
	CodeUsedByRecurring          = "used_by_recurring"
	CodeNotPayable               = "not_payable"
	CodeNothingToPay             = "nothing_to_pay"
	CodeFinalExists              = "final_exists"
	CodeCorrectionOnly           = "correction_invoice_only"
	CodeCorrectionTemplate       = "correction_template"
	CodeSyncNotConfigured        = "sync_not_configured"
	CodeZeroAmount               = "zero_amount"
	CodeAlreadyMatched           = "already_matched"
	CodeNotMatched               = "not_matched"
	CodeAutomaticTodo            = "automatic_todo"
	CodeIsDefault                = "is_default"
	CodeLastOfType               = "last_of_type"
	CodeStockNotTracked          = "stock_not_tracked"
	CodeGeneratedMove            = "generated_move"
	CodeNotVatPayer              = "not_vat_payer"
	CodeMissingTaxOffice         = "missing_tax_office"
	CodeLastOwner                = "last_owner"
	CodeOwnerOnly                = "owner_only"
	CodeAlreadyMember            = "already_member"
	CodeAlreadyJoined            = "already_joined"
	CodeInvitationExpired        = "invitation_expired"
	CodeInvitationEmail          = "invitation_email_mismatch"
	CodeInvitationRevoked        = "invitation_revoked"
	CodeRecurringEnded           = "recurring_ended"
	CodeTemplateMissing          = "template_missing"
	CodeWrongPassword            = "wrong_password"
	CodeNoVatNo                  = "no_vat_no"
	CodeFioToken                 = "fio_token_rejected"
	CodeFioTooMany               = "fio_too_many_transactions"
	CodeUnsupportedBackupVersion = "unsupported_backup_version"
	CodeCorruptBackup            = "corrupt_backup"
	CodeBackupTooLarge           = "backup_too_large"
)

// statusCodes are the generic codes of errors created without apiError.
var statusCodes = map[int]string{
	http.StatusBadRequest:            "bad_request",
	http.StatusUnauthorized:          "unauthorized",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not_found",
	http.StatusConflict:              "conflict",
	http.StatusGone:                  "gone",
	http.StatusRequestEntityTooLarge: "too_large",
	http.StatusUnsupportedMediaType:  "unsupported_media_type",
	http.StatusUnprocessableEntity:   CodeValidation,
	http.StatusTooManyRequests:       "rate_limited",
	http.StatusInternalServerError:   "internal",
	http.StatusBadGateway:            "upstream_unavailable",
	http.StatusGatewayTimeout:        "upstream_timeout",
}

func init() {
	// every error huma or a handler creates uses ErrorModel (also documents it in OpenAPI)
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		return newErrorModel(status, statusCodes[status], msg, errs...)
	}
}

func newErrorModel(status int, code, msg string, errs ...error) *ErrorModel {
	e := &ErrorModel{ErrorModel: huma.ErrorModel{Status: status, Title: http.StatusText(status), Detail: msg}, Code: code}
	for _, err := range errs {
		if err != nil {
			e.Add(err)
		}
	}
	return e
}

// apiError is an error with a specific machine-readable code, e.g.
// apiError(http.StatusConflict, CodeHasInvoices, "subject has invoices").
func apiError(status int, code, msg string, errs ...error) error {
	return newErrorModel(status, code, msg, errs...)
}
