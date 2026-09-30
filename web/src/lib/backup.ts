// Záloha a přenos účtu (SPEC §7.16): texty upozornění importu, velikosti, průběh.

export interface ImportWarningLike {
  code: string
  message: string
  count?: number
}

/** Česká věta k upozornění importu (podle `code`; neznámý kód → anglická `message`). */
export function importWarningText(w: ImportWarningLike): string {
  const n = w.count ?? 0
  switch (w.code) {
    case 'recurring_deactivated':
      return `${countText(n, 'pravidelná faktura byla vypnuta', 'pravidelné faktury byly vypnuty', 'pravidelných faktur bylo vypnuto')}, aby je dvě instance nevystavily dvakrát. Zapněte je, až budete připraveni.`
    case 'reminders_disabled':
      return 'Automatické upomínky jsou vypnuté. Zapněte je v nastavení E-maily a upomínky.'
    case 'paid_thanks_disabled':
      return 'Poděkování za platbu je vypnuté. Zapněte ho v nastavení E-maily a upomínky.'
    case 'webhooks_inactive':
      return `${countText(n, 'webhook je neaktivní', 'webhooky jsou neaktivní', 'webhooků je neaktivních')} a bez tajemství. Při zapnutí dostanete nové tajemství.`
    case 'bank_tokens_removed':
      return `Tokeny Fio API se nezálohují — zadejte je znovu u bankovních účtů (${n}).`
    case 'public_links_regenerated':
      return `Veřejné odkazy dokladů (${n}) mají nové adresy; dříve poslané odkazy nefungují.`
    case 'members_not_imported':
      return `Ostatní členové účtu (${n}) se nepřenáší — pozvěte je znovu v nastavení Tým.`
    case 'attachments_missing':
      return `${countText(n, 'příloha chyběla', 'přílohy chyběly', 'příloh chybělo')} v záloze a nebyla obnovena.`
    case 'orphans_skipped':
      return `Vynecháno ${n} záznamů odkazujících na data, která v záloze nejsou.`
    default:
      return w.message
  }
}

/** „1 faktura byla …“, „3 faktury byly …“, „5 faktur bylo …“. */
function countText(n: number, one: string, few: string, many: string): string {
  if (n === 1) return `1 ${one}`
  if (n >= 2 && n <= 4) return `${n} ${few}`
  return `${n} ${many}`
}

/** Velikost souboru česky: 512 B, 1,5 kB, 12,3 MB. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return ''
  if (bytes < 1024) return `${bytes} B`
  const units = ['kB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  const digits = v < 10 ? 1 : 0
  return `${v.toLocaleString('cs-CZ', { maximumFractionDigits: digits, minimumFractionDigits: digits })} ${units[i]}`
}

/** Procento průběhu (0–100) nebo `null`, když celková velikost není známá. */
export function progressPercent(loaded: number, total: number | null | undefined): number | null {
  if (!total || total <= 0) return null
  return Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
}

/** Je soubor pravděpodobně ZIP zálohy (podle názvu / typu)? */
export function looksLikeBackup(file: { name: string; type: string }): boolean {
  return /\.zip$/i.test(file.name) || file.type === 'application/zip' || file.type === 'application/x-zip-compressed'
}
