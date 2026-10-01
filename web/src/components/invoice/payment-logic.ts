// Pravidla dialogu „Přidat platbu“ (čisté funkce, testované v payment-logic.test.ts).

/**
 * Chyba ručně zadané částky platby, nebo `null`. Platba musí být nenulová a mít stejné znaménko
 * jako zbývající částka (u dobropisu je zbývající částka záporná → vrácení peněz = záporná platba).
 */
export function paymentAmountError(amount: number | null, remaining: number): string | null {
  if (amount === null || amount === 0) return 'Zadejte nenulovou částku'
  if (remaining > 0 && amount < 0) return 'Platba musí být kladná (zbývá doplatit)'
  if (remaining < 0 && amount > 0) return 'U dobropisu zadejte vrácenou částku se znaménkem minus'
  return null
}

/**
 * Předvolba „Vystavit konečnou fakturu“: jen když se platí celá zbývající částka. Při částečné
 * platbě by vyúčtování převzalo platbu a zbytek dlužilo na nové faktuře — to musí být vědomá volba.
 */
export function precheckFinalInvoice(amount: number | null, remaining: number): boolean {
  return amount !== null && amount !== 0 && amount === remaining
}

/** Zbytek, který po částečné platbě s vyúčtováním zůstane k úhradě na konečné faktuře (0 = nic). */
export function remainderOnFinalInvoice(amount: number | null, remaining: number): number {
  if (amount === null || amount === 0) return 0
  const rest = remaining - amount
  return Math.sign(rest) === Math.sign(remaining) ? rest : 0
}
