import fs from 'node:fs'
import { expect, linkFrom, mailbox, test } from '../lib/fixtures'
import { acct, confirm, docAction, fillLine, money, pickSubject } from '../lib/ui'

test.describe('invoice form', () => {
  test('create with multiple VAT rates, then edit', async ({ page, owner, api, isMobile }) => {
    await api.patch(`/api/accounts/${owner.slug}`, { vat_mode: 'vat_payer', vat_no: 'CZ12345679' })
    await api.subject({ name: 'Odběratel DPH s.r.o.' })

    await page.goto(acct(owner.slug, '/invoices/new'))
    await pickSubject(page, 'Odběratel DPH')

    await fillLine(page, 0, { name: 'Vývoj', quantity: '2', price: '1000' }, isMobile)
    await page.getByRole('button', { name: 'Přidat položku' }).click()
    await fillLine(page, 1, { name: 'Hosting', price: '500', vat: '12 %' }, isMobile)

    const recap = page.getByRole('table', { name: 'Rekapitulace DPH' })
    await expect(recap.getByRole('row', { name: /21 %/ })).toContainText(money(420))
    await expect(recap.getByRole('row', { name: /12 %/ })).toContainText(money(60))
    await expect(page.getByRole('main').getByRole('definition').last()).toHaveText(money(2980))

    await page.getByRole('button', { name: 'Uložit', exact: true }).click()
    await expect(page).toHaveURL(/\/invoices\/\d+$/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^\d{4}-\d{4}$/)
    await expect(page.getByRole('main')).toContainText(money(2980))

    await docAction(page, 'Upravit')
    await expect(page).toHaveURL(/\/edit$/)
    await fillLine(page, 0, { quantity: '3' }, isMobile)
    await expect(page.getByRole('main').getByRole('definition').last()).toHaveText(money(4190))
    await page.getByRole('button', { name: 'Uložit změny' }).click()
    await expect(page).toHaveURL(/\/invoices\/\d+$/)
    await expect(page.getByRole('main')).toContainText(money(4190))
  })
})

test.describe('invoice lifecycle', () => {
  test('send e-mail with PDF, public link page, PDF download', async ({ page, owner, api }) => {
    await api.bankAccount()
    const client = await api.subject({ name: 'Klient Mail s.r.o.' })
    const inv = await api.invoice(client.id, { private_note: 'TAJNÁ INTERNÍ POZNÁMKA' })

    await page.goto(acct(owner.slug, `/invoices/${inv.id}`))
    await page.getByRole('main').getByRole('button', { name: 'Odeslat e-mailem' }).click()
    const dlg = page.getByRole('dialog', { name: `Odeslat ${inv.number} e-mailem` })
    await expect(dlg.getByRole('radio', { name: 'Čeština' })).toBeChecked()
    await expect(dlg).toContainText(client.email)
    await expect(dlg.getByLabel('Předmět')).toHaveValue(new RegExp(inv.number))
    await expect(dlg.getByRole('switch', { name: 'PDF dokladu' })).toBeChecked()
    await dlg.getByRole('button', { name: 'Odeslat', exact: true }).click()
    await expect(dlg).toBeHidden()
    await expect(page.getByRole('main').getByText('Odeslaná').first()).toBeVisible()

    const mail = await mailbox.waitFor(client.email, new RegExp(inv.number))
    expect(mail.attachments.map((a) => a.contentType)).toContain('application/pdf')
    expect(mail.attachments.find((a) => a.contentType === 'application/pdf')!.size).toBeGreaterThan(1000)
    const publicLink = linkFrom(mail, '/p/')

    // public page: payment QR, no private data, PDF download
    await page.context().clearCookies()
    await page.goto(publicLink)
    await expect(page.getByText(`č. ${inv.number}`)).toBeVisible()
    await expect(page.getByRole('img', { name: /QR Platba/ })).toBeVisible()
    await expect(page.locator('body')).not.toContainText('TAJNÁ INTERNÍ POZNÁMKA')
    await expect(page.getByRole('link', { name: 'Nastavení' })).toHaveCount(0)
    // "Stáhnout PDF" opens the PDF in a new tab; fetch its target instead of relying on the PDF viewer
    const href = await page.getByRole('button', { name: 'Stáhnout PDF' }).getAttribute('href')
    const res = await page.request.get(new URL(href!, page.url()).toString())
    expect(res.headers()['content-type']).toContain('application/pdf')
    expect((await res.body()).subarray(0, 5).toString()).toBe('%PDF-')
  })

  test('payment makes it paid; correction and duplicate', async ({ page, owner, api }) => {
    const client = await api.subject()
    const inv = await api.invoice(client.id)

    await page.goto(acct(owner.slug, `/invoices/${inv.id}`))
    await page.getByRole('main').getByRole('button', { name: 'Přidat platbu' }).first().click()
    const dlg = page.getByRole('dialog', { name: 'Přidat platbu' })
    await expect(dlg.getByLabel('Částka (CZK)')).toHaveValue('3000,00')
    await dlg.getByRole('button', { name: 'Přidat platbu' }).click()
    await expect(dlg).toBeHidden()
    await expect(page.getByRole('main').getByText('Uhrazená').first()).toBeVisible()
    await expect(page.getByRole('main').getByRole('button', { name: 'Přidat platbu' })).toHaveCount(0)

    await docAction(page, 'Vystavit opravný doklad')
    await confirm(page, 'Vystavit opravný doklad', 'Vystavit')
    await expect(page).toHaveURL(/\/invoices\/\d+\/edit$/)
    const correctionId = Number(page.url().match(/invoices\/(\d+)\/edit/)![1])
    const correction = await api.get(api.acct(`/invoices/${correctionId}`))
    expect(correction).toMatchObject({ document_type: 'correction', related_id: inv.id, total: -300000 })

    await page.goto(acct(owner.slug, `/invoices/${inv.id}`))
    await docAction(page, 'Duplikovat')
    await expect(page).toHaveURL(/\/invoices\/\d+\/edit$/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^Upravit /)
    const copyId = Number(page.url().match(/invoices\/(\d+)\/edit/)![1])
    expect(copyId).not.toBe(inv.id)
    const copy = await api.get(api.acct(`/invoices/${copyId}`))
    expect(copy).toMatchObject({ document_type: 'invoice', status: 'open', total: inv.total })
  })

  test('proforma payment issues the final invoice', async ({ page, owner, api }) => {
    const client = await api.subject()
    const proforma = await api.invoice(client.id, { document_type: 'proforma' })

    await page.goto(acct(owner.slug, `/invoices/${proforma.id}`))
    await expect(page.getByRole('main')).toContainText('Zálohová faktura')
    await page.getByRole('main').getByRole('button', { name: 'Přidat platbu' }).first().click()
    const dlg = page.getByRole('dialog', { name: 'Přidat platbu' })
    await expect(dlg.getByRole('switch', { name: 'Vystavit konečnou fakturu' })).toBeChecked()
    await dlg.getByRole('button', { name: 'Přidat platbu' }).click()
    await expect(dlg).toBeHidden()

    let finals: { id: number; status: string; related_id?: number }[] = []
    await expect
      .poll(async () => {
        const list = await api.get(api.acct('/invoices?document_type=invoice'))
        finals = list.items.filter((i: { related_id?: number }) => i.related_id === proforma.id)
        return finals.map((i) => i.status)
      })
      .toEqual(['paid'])
    await page.goto(acct(owner.slug, `/invoices/${finals[0].id}`))
    await expect(page.getByRole('main').getByText('Uhrazená').first()).toBeVisible()
  })

  test('mark as sent, PDF download, cancel and undo, lock and unlock', async ({ page, owner, api, isMobile }) => {
    const client = await api.subject()
    const inv = await api.invoice(client.id)
    await page.goto(acct(owner.slug, `/invoices/${inv.id}`))

    await docAction(page, 'Označit jako odeslanou')
    await expect(page.getByRole('main').getByText('Odeslaná').first()).toBeVisible()

    if (!isMobile) {
      const [download] = await Promise.all([page.waitForEvent('download'), docAction(page, 'Stáhnout PDF')])
      expect(download.suggestedFilename()).toBe(`faktura-${inv.number}.pdf`)
      const file = await download.path()
      expect(fs.readFileSync(file).subarray(0, 5).toString()).toBe('%PDF-')
    }

    await docAction(page, 'Stornovat')
    await confirm(page, 'Stornovat doklad?', 'Stornovat')
    await expect(page.getByRole('main').getByText('Stornovaná').first()).toBeVisible()
    await docAction(page, 'Obnovit (zrušit storno)')
    await expect(page.getByRole('main').getByText('Odeslaná').first()).toBeVisible()

    await docAction(page, 'Zamknout')
    await expect(page.getByRole('main').getByText('Zamčeno')).toBeVisible()
    await expect(page.getByRole('main').getByRole('button', { name: 'Upravit' })).toHaveCount(0)
    // a locked document cannot be edited through the API either
    await expect(api.patch(api.acct(`/invoices/${inv.id}`), { note: 'x' })).rejects.toThrow(/409/)
    await docAction(page, 'Odemknout')
    await expect(page.getByRole('main').getByText('Zamčeno')).toHaveCount(0)
    await api.patch(api.acct(`/invoices/${inv.id}`), { note: 'po odemčení' })
  })
})
