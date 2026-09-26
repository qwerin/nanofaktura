package billing

import (
	"time"

	"github.com/qwerin/nanofaktura/internal/model"
)

// StatusOverdue is the derived status of an open/sent document past its due date.
// It is never stored.
const StatusOverdue = "overdue"

// IsOverdue reports whether a document with stored status and due date is
// overdue on today (all dates "YYYY-MM-DD", compared lexically).
func IsOverdue(status, dueOn, today string) bool {
	return (status == model.StatusOpen || status == model.StatusSent) && dueOn != "" && dueOn < today
}

// EffectiveStatus is the status shown to clients: the stored one, or
// "overdue" when IsOverdue.
func EffectiveStatus(status, dueOn, today string) string {
	if IsOverdue(status, dueOn, today) {
		return StatusOverdue
	}
	return status
}

// IsPaid reports whether paid covers total (for negative totals, i.e.
// refunds on corrections, paid must reach down to total). Without any
// payment a document is never paid.
func IsPaid(total, paid int64, payments int) bool {
	if payments == 0 {
		return false
	}
	if total < 0 {
		return paid <= total
	}
	return paid >= total
}

// PaymentStatus recomputes the stored status of an open/sent/paid document
// after its payments or total changed: paid when covered, otherwise sent (if
// it was ever sent) or open. Documents in other states keep their status.
func PaymentStatus(status string, total, paid int64, payments int, sent bool) string {
	switch status {
	case model.StatusOpen, model.StatusSent, model.StatusPaid:
	default:
		return status
	}
	switch {
	case IsPaid(total, paid, payments):
		return model.StatusPaid
	case sent:
		return model.StatusSent
	default:
		return model.StatusOpen
	}
}

// Invoice actions (POST …/invoices/{id}/actions/{action}).
const (
	ActionMarkAsSent          = "mark_as_sent"
	ActionCancel              = "cancel"
	ActionUndoCancel          = "undo_cancel"
	ActionMarkAsUncollectible = "mark_as_uncollectible"
	ActionUndoUncollectible   = "undo_uncollectible"
	ActionLock                = "lock"
	ActionUnlock              = "unlock"
)

// Actions lists every action in documentation order.
var Actions = []string{
	ActionMarkAsSent, ActionCancel, ActionUndoCancel, ActionMarkAsUncollectible,
	ActionUndoUncollectible, ActionLock, ActionUnlock,
}

// State is the part of a document the state machine reads and writes.
type State struct {
	Status          string // stored status (never "overdue")
	SentAt          *time.Time
	CancelledAt     *time.Time
	UncollectibleAt *time.Time
	LockedAt        *time.Time
	HasPayments     bool
}

// TransitionError is a human-readable reason why an action is not allowed.
type TransitionError struct{ Msg string }

func (e *TransitionError) Error() string { return e.Msg }

func refuse(msg string) (State, error) { return State{}, &TransitionError{Msg: msg} }

// ApplyAction runs action on s at now and returns the new state, or a
// *TransitionError when the transition is not allowed (SPEC §4.5):
//
//	mark_as_sent           open → sent (sent_at = now)
//	cancel                 open|sent without payments → cancelled
//	undo_cancel            cancelled → sent if sent_at is set, else open
//	mark_as_uncollectible  open|sent → uncollectible
//	undo_uncollectible     uncollectible → sent/open (by sent_at)
//	lock / unlock          any → sets / clears locked_at
func ApplyAction(s State, action string, now time.Time) (State, error) {
	restored := model.StatusOpen
	if s.SentAt != nil {
		restored = model.StatusSent
	}
	switch action {
	case ActionMarkAsSent:
		if s.Status != model.StatusOpen {
			return refuse("only an open document can be marked as sent (current status: " + s.Status + ")")
		}
		s.Status, s.SentAt = model.StatusSent, &now
	case ActionCancel:
		if s.Status != model.StatusOpen && s.Status != model.StatusSent {
			return refuse("only an open or sent document can be cancelled (current status: " + s.Status + ")")
		}
		if s.HasPayments {
			return refuse("a document with payments cannot be cancelled; delete the payments first")
		}
		s.Status, s.CancelledAt = model.StatusCancelled, &now
	case ActionUndoCancel:
		if s.Status != model.StatusCancelled {
			return refuse("the document is not cancelled")
		}
		s.Status, s.CancelledAt = restored, nil
	case ActionMarkAsUncollectible:
		if s.Status != model.StatusOpen && s.Status != model.StatusSent {
			return refuse("only an open or sent document can be marked as uncollectible (current status: " + s.Status + ")")
		}
		s.Status, s.UncollectibleAt = model.StatusUncollectible, &now
	case ActionUndoUncollectible:
		if s.Status != model.StatusUncollectible {
			return refuse("the document is not marked as uncollectible")
		}
		s.Status, s.UncollectibleAt = restored, nil
	case ActionLock:
		s.LockedAt = &now
	case ActionUnlock:
		s.LockedAt = nil
	default:
		return refuse("unknown action " + action)
	}
	return s, nil
}
