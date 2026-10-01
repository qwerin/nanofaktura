import type { Locator, Page } from '@playwright/test'
import { expect, test } from '../lib/fixtures'
import { acct, docAction, money, toast } from '../lib/ui'

/** Řádek součtů („Uhrazeno“, „Zbývá uhradit“ …) v panelu rekapitulace. */
function totalsRow(page: Page, label: string): Locator {
  return page
    .getByRole('main')
    .locator('dl > div')
    .filter({ has: page.getByRole('term').getByText(label, { exact: true }) })
    .first()
}

test.describe('advances (VAT payer)', () => {
  test('partial proforma payment → tax document; final invoice deducts the deposit', async ({ page, owner, api }) => {
    await api.patch(`/api/accounts/${owner.slug}`, { vat_mode: 'vat_payer', vat_no: 'CZ12345679' })
    const client = await api.subject({ name: 'Zálohový klient s.r.o.' })
    const proforma = await api.invoice(client.id, {
      document_type: 'proforma',
      lines: [{ name: 'Vývoj e-shopu', quantity: '1', unit_price: 100000, vat_rate_bps: 2100 }],
    })
    expect(proforma.total).toBe(121000)

    // částečná platba 400 Kč: vyúčtování se nepředvolí, vznikne daňový doklad k platbě
    await page.goto(acct(owner.slug, `/invoices/${proforma.id}`))
    await expect(page.getByRole('main')).toContainText(money(1210))
    await page.getByRole('main').getByRole('button', { name: 'Přidat platbu' }).first().click()
    const dlg = page.getByRole('dialog', { name: 'Přidat platbu' })
    await expect(dlg).toContainText('automaticky vystaví daňový doklad')
    const finalSwitch = dlg.getByRole('switch', { name: 'Vystavit konečnou fakturu' })
    await expect(finalSwitch).toBeChecked()
    await dlg.getByLabel('Částka (CZK)').fill('400')
    await expect(finalSwitch).not.toBeChecked()
    await dlg.getByRole('button', { name: 'Přidat platbu' }).click()
    await expect(dlg).toBeHidden()
    await toast(page, 'vystaven daňový doklad k přijaté platbě')

    const related = page.getByRole('main').getByRole('link', { name: /^ZD\d{4}-\d{4}$/ }).first()
    await expect(related).toBeVisible()
    const taxDocNumber = (await related.textContent())!.trim()
    await expect(totalsRow(page, 'Zbývá uhradit')).toContainText(money(810))

    // vyúčtování z menu akcí
    await docAction(page, 'Vystavit vyúčtování')
    const finalDlg = page.getByRole('dialog', { name: 'Vystavit vyúčtování' })
    await expect(finalDlg.getByLabel('Datum zdanitelného plnění')).toBeVisible()
    await finalDlg.getByRole('button', { name: 'Vystavit', exact: true }).click()
    await expect(finalDlg).toBeHidden()
    await expect(page).not.toHaveURL(new RegExp(`/invoices/${proforma.id}$`))
    await expect(page).toHaveURL(/\/invoices\/\d+$/)

    // konečná faktura: 1 210 Kč, uhrazeno 400 Kč (převzatá platba), zbývá 810 Kč, odpočet zálohy
    const main = page.getByRole('main')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^\d{4}-\d{4}$/)
    await expect(main.getByText('Celkem', { exact: true }).first()).toBeVisible()
    await expect(main).toContainText(money(1210))
    await expect(totalsRow(page, 'Uhrazeno')).toContainText(money(400))
    await expect(totalsRow(page, 'Zbývá uhradit')).toContainText(money(810))
    await expect(main.getByRole('heading', { name: 'Odpočet záloh' })).toBeVisible()
    await expect(main.getByRole('link', { name: taxDocNumber })).toBeVisible()
    await expect(main.getByRole('button', { name: 'Smazat platbu' })).toBeDisabled()
    const finalNumber = (await page.getByRole('heading', { level: 1 }).textContent())!.trim()

    // proforma je vyúčtovaná: odkaz na fakturu, žádné další platby
    await main.getByRole('link', { name: 'Zobrazit doklad' }).click()
    await expect(page).toHaveURL(new RegExp(`/invoices/${proforma.id}$`))
    await expect(page.getByRole('main').getByText('Vyúčtováno fakturou').first()).toBeVisible()
    await expect(page.getByRole('main').getByRole('link', { name: finalNumber }).first()).toBeVisible()
    await expect(page.getByRole('main').getByText('Uhrazená').first()).toBeVisible()
    await expect(page.getByRole('main').getByRole('button', { name: 'Přidat platbu' })).toHaveCount(0)
  })
})
