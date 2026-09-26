// Package numbering implements document number series (SPEC §4.3).
//
// A format is literal text with placeholders {YYYY}, {YY}, {MM} and {N}…{NNNNNN}
// (the count of N is the zero-padding width). Every format must contain at
// least one {N…}. The counter period is derived from the date placeholders:
// "YYYY-MM" when the format contains {MM}, "YYYY" when it contains only a
// year, "" otherwise — so a series restarts automatically.
package numbering

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/model"
)

// MaxFormatLength is the maximum length of a format string.
const MaxFormatLength = 50

// DateLayout is the layout of issued_on dates.
const DateLayout = "2006-01-02"

var (
	// ErrInvalidFormat wraps every format validation error.
	ErrInvalidFormat = errors.New("invalid number format")
	// ErrNoFormat is returned by Next when the account has no default format for the document type.
	ErrNoFormat = errors.New("no default number format for document type")
	// ErrInvalidDate is returned for an issued_on that is not YYYY-MM-DD.
	ErrInvalidDate = errors.New("invalid date, expected YYYY-MM-DD")
)

type token struct {
	lit string // literal text (when kind == 0)
	// kind: 0 literal, 'Y' {YYYY}, 'y' {YY}, 'M' {MM}, 'N' {N…}
	kind  byte
	width int // zero-pad width of {N…}
}

func parse(format string) ([]token, error) {
	if format == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidFormat)
	}
	if len(format) > MaxFormatLength {
		return nil, fmt.Errorf("%w: longer than %d characters", ErrInvalidFormat, MaxFormatLength)
	}
	var toks []token
	hasN := false
	rest := format
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		closing := strings.IndexByte(rest, '}')
		if open < 0 {
			if closing >= 0 {
				return nil, fmt.Errorf("%w: unexpected '}'", ErrInvalidFormat)
			}
			toks = append(toks, token{lit: rest})
			break
		}
		if closing >= 0 && closing < open {
			return nil, fmt.Errorf("%w: unexpected '}'", ErrInvalidFormat)
		}
		if open > 0 {
			toks = append(toks, token{lit: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return nil, fmt.Errorf("%w: unclosed '{'", ErrInvalidFormat)
		}
		name := rest[open+1 : open+end]
		switch {
		case name == "YYYY":
			toks = append(toks, token{kind: 'Y'})
		case name == "YY":
			toks = append(toks, token{kind: 'y'})
		case name == "MM":
			toks = append(toks, token{kind: 'M'})
		case name != "" && len(name) <= 6 && strings.Trim(name, "N") == "":
			toks = append(toks, token{kind: 'N', width: len(name)})
			hasN = true
		default:
			return nil, fmt.Errorf("%w: unknown placeholder {%s} (allowed {YYYY}, {YY}, {MM}, {N}…{NNNNNN})", ErrInvalidFormat, name)
		}
		rest = rest[open+end+1:]
	}
	if !hasN {
		return nil, fmt.Errorf("%w: must contain a sequence placeholder {N}…{NNNNNN}", ErrInvalidFormat)
	}
	return toks, nil
}

// Validate checks a format string; errors wrap ErrInvalidFormat.
func Validate(format string) error {
	_, err := parse(format)
	return err
}

// Period returns the counter period of format for date: "2026-03" if the
// format contains {MM}, "2026" if it contains {YYYY} or {YY}, else "".
// An invalid format yields "".
func Period(format string, date time.Time) string {
	toks, err := parse(format)
	if err != nil {
		return ""
	}
	year, month := false, false
	for _, t := range toks {
		switch t.kind {
		case 'Y', 'y':
			year = true
		case 'M':
			month = true
		}
	}
	switch {
	case month:
		return date.Format("2006-01")
	case year:
		return date.Format("2006")
	default:
		return ""
	}
}

// Render formats sequence number n with date, e.g. Render("{YYYY}-{NNNN}", d, 7) = "2026-0007".
// Numbers wider than the padding are not truncated. An invalid format yields an error.
func Render(format string, date time.Time, n int64) (string, error) {
	toks, err := parse(format)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range toks {
		switch t.kind {
		case 0:
			b.WriteString(t.lit)
		case 'Y':
			fmt.Fprintf(&b, "%04d", date.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", date.Year()%100)
		case 'M':
			fmt.Fprintf(&b, "%02d", int(date.Month()))
		case 'N':
			s := strconv.FormatInt(n, 10)
			if pad := t.width - len(s); pad > 0 {
				b.WriteString(strings.Repeat("0", pad))
			}
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

// ParseDate parses a YYYY-MM-DD date (errors wrap ErrInvalidDate).
func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDate, s)
	}
	return d, nil
}

// DefaultFormat loads the account's default format for docType (ErrNoFormat if none).
func DefaultFormat(db *gorm.DB, accountID uint, docType string) (*model.NumberFormat, error) {
	var nf model.NumberFormat
	err := db.Where("account_id = ? AND document_type = ? AND is_default = ?", accountID, docType, true).
		Order("id").First(&nf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w %q", ErrNoFormat, docType)
	}
	if err != nil {
		return nil, err
	}
	return &nf, nil
}

// Next assigns the next number of the account's default series for docType
// and date issuedOn (YYYY-MM-DD): it locks the period counter (SELECT … FOR
// UPDATE on Postgres; SQLite is serialized by its single connection),
// increments it and returns the rendered number.
//
// Call it with the caller's transaction so the counter only moves when the
// document is actually stored.
func Next(tx *gorm.DB, accountID uint, docType string, issuedOn string) (string, error) {
	date, err := ParseDate(issuedOn)
	if err != nil {
		return "", err
	}
	nf, err := DefaultFormat(tx, accountID, docType)
	if err != nil {
		return "", err
	}
	if err := Validate(nf.Format); err != nil {
		return "", err
	}
	period := Period(nf.Format, date)

	// Ensure the counter row exists without racing concurrent creators,
	// then lock it for the rest of the transaction.
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.NumberCounter{NumberFormatID: nf.ID, Period: period}).Error; err != nil {
		return "", err
	}
	var c model.NumberCounter
	if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("number_format_id = ? AND period = ?", nf.ID, period).First(&c).Error; err != nil {
		return "", err
	}
	c.LastNumber++
	if err := tx.Model(&c).Update("last_number", c.LastNumber).Error; err != nil {
		return "", err
	}
	return Render(nf.Format, date, c.LastNumber)
}

// Preview returns the number format nf would assign next for issuedOn
// (YYYY-MM-DD) without moving the counter.
func Preview(db *gorm.DB, nf *model.NumberFormat, issuedOn string) (string, error) {
	date, err := ParseDate(issuedOn)
	if err != nil {
		return "", err
	}
	if err := Validate(nf.Format); err != nil {
		return "", err
	}
	var last int64
	err = db.Model(&model.NumberCounter{}).
		Where("number_format_id = ? AND period = ?", nf.ID, Period(nf.Format, date)).
		Pluck("last_number", &last).Error
	if err != nil {
		return "", err
	}
	return Render(nf.Format, date, last+1)
}
