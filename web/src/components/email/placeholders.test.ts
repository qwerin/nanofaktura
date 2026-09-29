import { describe, expect, it } from 'vitest'
import {
  cleanBlankLines,
  emailVars,
  isEmailLike,
  renderEmail,
  sampleEmailInvoice,
  sendableKinds,
  splitAddresses,
} from './placeholders'

const inv = { ...sampleEmailInvoice('2026-09-01'), due_on: '2026-09-15', paid_amount: 210_000 }
const opts = { accountName: 'Moje firma', lang: 'cs' as const, origin: 'https://f.example', today: '2026-09-20' }

describe('emailVars', () => {
  it('formats values in the invoice language', () => {
    const v = emailVars(inv, opts)
    expect(v.number).toBe('2026-0001')
    expect(v.document).toBe('faktura')
    expect(v.total.replace(/\s/g, ' ')).toBe('12 100,00 Kč')
    expect(v.remaining.replace(/\s/g, ' ')).toBe('10 000,00 Kč')
    expect(v.due_on).toBe('15. 9. 2026')
    expect(v.days_overdue).toBe('5')
    expect(v.public_url).toBe('https://f.example/p/ukazka')
    expect(v.payment_info).toBe('Číslo účtu: 123456789/0800\nIBAN: CZ6508000000000123456789\nVariabilní symbol: 20260001')
  })
  it('uses English names and no payment info for cash', () => {
    const v = emailVars({ ...inv, payment_method: 'cash', document_type: 'proforma' }, { ...opts, lang: 'en' })
    expect(v.document_title).toBe('Proforma invoice')
    expect(v.payment_info).toBe('')
    expect(v.due_on).toBe('15 Sept 2026')
  })
  it('is not overdue before the due date', () => {
    expect(emailVars(inv, { ...opts, today: '2026-09-10' }).days_overdue).toBe('0')
  })
})

describe('renderEmail', () => {
  it('renders placeholders, appends the signature and collapses blank lines', () => {
    const out = renderEmail(
      { subject: ' Faktura {number} – {account_name} ', body: 'Dobrý den,\n\n{payment_info}\n\n\nZbývá {remaining}. {unknown}\n' },
      { number: '1', account_name: 'Firma', payment_info: '', remaining: '5 Kč' },
      'Jan Novák',
    )
    expect(out.subject).toBe('Faktura 1 – Firma')
    expect(out.body).toBe('Dobrý den,\n\nZbývá 5 Kč. {unknown}\n\nJan Novák')
  })
  it('cleanBlankLines trims trailing spaces', () => {
    expect(cleanBlankLines('a  \n\n\n\nb\t\n')).toBe('a\n\nb')
  })
})

describe('sendableKinds', () => {
  it('offers kinds by state', () => {
    expect(sendableKinds({ status: 'open', document_type: 'invoice' })).toEqual(['invoice'])
    expect(sendableKinds({ status: 'overdue', document_type: 'invoice' })).toEqual(['invoice', 'reminder'])
    expect(sendableKinds({ status: 'paid', document_type: 'proforma' })).toEqual(['invoice', 'paid_thanks'])
    expect(sendableKinds({ status: 'overdue', document_type: 'correction' })).toEqual(['invoice'])
  })
})

describe('addresses', () => {
  it('splits and validates', () => {
    expect(splitAddresses(' a@b.cz, c@d.cz;e@f.cz\n')).toEqual(['a@b.cz', 'c@d.cz', 'e@f.cz'])
    expect(isEmailLike('a@b.cz')).toBe(true)
    expect(isEmailLike('a@b')).toBe(false)
  })
})
