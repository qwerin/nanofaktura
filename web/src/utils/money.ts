// Formátování haléřových hodnot pro zobrazení v UI

/** Převede haléře na čitelný řetězec, např. 150050 → "1 500,50 Kč" */
export function formatKc(hal: number): string {
  const kc = hal / 100
  return new Intl.NumberFormat('cs-CZ', {
    style: 'currency',
    currency: 'CZK',
    minimumFractionDigits: 2,
  }).format(kc)
}

/** Zobrazí decimal string množství jako číslo pro UI */
export function formatQty(qty: string): string {
  const n = parseFloat(qty)
  if (isNaN(n)) return qty
  return n.toLocaleString('cs-CZ')
}
