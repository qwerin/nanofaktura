// Small UI helpers shared by the specs (accessible roles/labels only).
import { expect, type Locator, type Page } from '@playwright/test'

/** Czech money text regex: money(2980) matches "2 980,00 Kč" with any (nb)space. */
export function money(amount: number, currency = 'Kč'): RegExp {
  const [int, dec] = amount.toFixed(2).split('.')
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, '\\s')
  return new RegExp(`${grouped},${dec}\\s${currency}`)
}

/** Opens a (base-ui) select by its trigger and picks an option. */
export async function choose(page: Page, trigger: Locator, option: string | RegExp) {
  await trigger.click()
  await page.getByRole('option', { name: option }).first().click()
}

/** Picks a contact in the invoice/expense subject picker. */
export async function pickSubject(page: Page, name: string, label = 'Odběratel') {
  await page.getByRole('button', { name: label, exact: true }).click()
  const dlg = page.getByRole('dialog', { name: label })
  await dlg.getByRole('combobox').fill(name)
  await dlg.getByRole('option', { name: new RegExp(name) }).first().click()
  await expect(dlg).toBeHidden()
}

export async function toast(page: Page, text: string | RegExp) {
  await expect(page.locator('[data-sonner-toast]').filter({ hasText: text }).first()).toBeVisible()
}

/**
 * The visible element(s) with `text` inside `scope`. Lists render a desktop table and
 * mobile cards side by side (one hidden by CSS), so plain getByText would match twice.
 */
export function shown(scope: Locator | Page, text: string | RegExp): Locator {
  return scope.getByText(text, { exact: typeof text === 'string' }).filter({ visible: true }).first()
}

/** Account-relative URL. */
export const acct = (slug: string, path = '') => `/a/${slug}${path}`

/**
 * Runs a document action: a visible header button if there is one, otherwise the
 * "Další akce" menu (dropdown on desktop, "Akce" sheet with buttons on mobile).
 */
export async function docAction(page: Page, item: string | RegExp) {
  const direct = page.getByRole('main').getByRole('button', { name: item, exact: typeof item === 'string' })
  if (await direct.first().isVisible()) {
    await direct.first().click()
    return
  }
  await page.getByRole('button', { name: 'Další akce' }).click()
  const menuItem = page.getByRole('menuitem', { name: item })
  const sheetButton = page.getByRole('dialog', { name: 'Akce' }).getByRole('button', { name: item })
  await menuItem.or(sheetButton).first().click()
}

export interface LineInput {
  name?: string
  quantity?: string
  unit?: string
  price?: string
  vat?: string
}

/**
 * Fills line `index` (0-based) of the lines editor: a table on desktop,
 * collapsible cards (one open at a time) on mobile.
 */
export async function fillLine(page: Page, index: number, line: LineInput, mobile: boolean) {
  const n = index + 1
  if (!mobile) {
    const row = page.getByRole('row', { name: new RegExp(`^Položka ${n}:`) })
    if (line.name !== undefined) await page.getByLabel(`Položka ${n}: název`).fill(line.name)
    if (line.quantity !== undefined) await page.getByLabel(`Položka ${n}: množství`).fill(line.quantity)
    if (line.unit !== undefined) await page.getByLabel(`Položka ${n}: jednotka`).fill(line.unit)
    if (line.price !== undefined) await page.getByLabel(`Položka ${n}: cena za jednotku`).fill(line.price)
    if (line.vat !== undefined) await choose(page, row.getByLabel('Sazba DPH'), line.vat)
    return
  }
  const headerRe = new RegExp(`^${n} `)
  const card = page.getByRole('main').getByRole('listitem').filter({ has: page.getByRole('button', { name: headerRe }) }).first()
  const header = card.getByRole('button', { name: headerRe }).first()
  if ((await header.getAttribute('aria-expanded')) !== 'true') await header.click()
  if (line.name !== undefined) await card.getByLabel('Název položky').fill(line.name)
  if (line.quantity !== undefined) await card.getByLabel('Množství').fill(line.quantity)
  if (line.unit !== undefined) await card.getByLabel('Jednotka').fill(line.unit)
  if (line.price !== undefined) await card.getByLabel('Cena za jednotku').fill(line.price)
  if (line.vat !== undefined) await choose(page, card.getByLabel('Sazba DPH'), line.vat)
}

/** Confirms the confirmation dialog titled `title` with its `button`. */
export async function confirm(page: Page, title: string | RegExp, button: string) {
  const dlg = page.getByRole('dialog', { name: title }).or(page.getByRole('alertdialog', { name: title }))
  await dlg.getByRole('button', { name: button, exact: true }).click()
  await expect(dlg).toBeHidden()
}
