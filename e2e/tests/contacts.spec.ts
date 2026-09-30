import { expect, makeIco, test } from '../lib/fixtures'
import { acct, confirm, docAction, shown } from '../lib/ui'

test('contact: create from ARES, edit, search, delete', async ({ page, owner, api }) => {
  await api.subject({ name: 'Alfa Stavby a.s.' })
  await api.subject({ name: 'Beta Obchod s.r.o.' })
  const ico = makeIco('2')

  await page.goto(acct(owner.slug, '/subjects/new'))
  await page.getByLabel('IČO').fill(ico)
  await page.getByRole('button', { name: 'Načíst z ARES' }).click()
  await expect(page.getByLabel('Název firmy / jméno')).toHaveValue(`Firma ${ico} s.r.o.`)
  await expect(page.getByLabel('DIČ')).toHaveValue(`CZ${ico}`)
  await expect(page.getByLabel('Město')).toHaveValue(/Praha/)
  await expect(page.getByLabel('PSČ')).toHaveValue(/110\s?00/)
  await page.getByLabel('E-mail', { exact: true }).fill('fakturace@ares-firma.test')
  await page.getByRole('button', { name: 'Vytvořit kontakt' }).click()
  await expect(page).toHaveURL(/\/subjects\/\d+$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Firma ${ico} s.r.o.`)
  await expect(page.getByRole('main')).toContainText(ico)

  await docAction(page, 'Upravit')
  await page.getByLabel('Název firmy / jméno').fill('Gama Software s.r.o.')
  await page.getByRole('button', { name: /^Uložit/ }).click()
  await expect(page).toHaveURL(/\/subjects\/\d+$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Gama Software s.r.o.')

  await page.goto(acct(owner.slug, '/subjects'))
  const main = page.getByRole('main')
  await expect(main.getByText('3 kontakty')).toBeVisible()
  await main.getByRole('searchbox', { name: 'Hledat kontakty' }).fill('beta')
  await expect(shown(main, 'Beta Obchod s.r.o.')).toBeVisible()
  await expect(shown(main, 'Alfa Stavby a.s.')).toHaveCount(0)
  await expect(shown(main, 'Gama Software s.r.o.')).toHaveCount(0)
  // search folds diacritics and matches the IČO too
  await main.getByRole('searchbox', { name: 'Hledat kontakty' }).fill(ico)
  await expect(shown(main, 'Gama Software s.r.o.')).toBeVisible()
  await shown(main, 'Gama Software s.r.o.').click()

  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Gama Software s.r.o.')
  await docAction(page, 'Smazat')
  await confirm(page, 'Smazat kontakt?', 'Smazat kontakt')
  await expect(page).toHaveURL(/\/subjects$/)
  await expect(main.getByText('2 kontakty')).toBeVisible()
})

test('contact: unknown IČO in ARES shows a message', async ({ page, owner }) => {
  await page.goto(acct(owner.slug, '/subjects/new'))
  await page.getByLabel('IČO').fill(makeIco('9')) // the fake ARES answers 404 for IČO starting with 9
  await page.getByRole('button', { name: 'Načíst z ARES' }).click()
  await expect(page.getByText('Subjekt s tímto IČO v ARES není.')).toBeVisible()
  await expect(page.getByLabel('Název firmy / jméno')).toHaveValue('')
})

test('price list item with stock: receipt and invoice write-off', async ({ page, owner, api }) => {
  await page.goto(acct(owner.slug, '/price-items/new'))
  await page.getByLabel('Název', { exact: true }).fill('Kabel USB-C')
  await page.getByLabel('Cena za jednotku bez DPH').fill('199')
  await page.getByRole('switch', { name: 'Sledovat skladové zásoby' }).click()
  await page.getByLabel('Počáteční stav (ks)').fill('10')
  await page.getByRole('button', { name: 'Přidat do ceníku' }).click()
  await expect(page).toHaveURL(/\/price-items\/\d+$/)
  const itemId = Number(page.url().match(/price-items\/(\d+)$/)![1])
  const main = page.getByRole('main')
  await expect(main.getByText('10 ks', { exact: true })).toBeVisible()

  await main.getByRole('button', { name: 'Příjem' }).first().click()
  const dlg = page.getByRole('dialog', { name: 'Příjem na sklad' })
  await dlg.getByLabel('Množství (ks)').fill('5')
  await dlg.getByRole('button', { name: 'Přijmout' }).click()
  await expect(dlg).toBeHidden()
  await expect(main.getByText('15 ks', { exact: true })).toBeVisible()

  // an invoice with the item writes the stock off
  const client = await api.subject()
  await api.invoice(client.id, { lines: [{ name: 'Kabel USB-C', price_item_id: itemId, quantity: '3', unit_price: 19900, vat_rate_bps: 0 }] })
  await page.reload()
  await expect(main.getByText('12 ks', { exact: true })).toBeVisible()
  await expect(main.getByText(/−3 ks/)).toBeVisible()

  await page.goto(acct(owner.slug, '/price-items'))
  await expect(shown(main, 'Kabel USB-C')).toBeVisible()
})
