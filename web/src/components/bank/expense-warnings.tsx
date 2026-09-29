import { TriangleAlertIcon } from 'lucide-react'

/**
 * Varování z registru plátců DPH u nákladu (nespolehlivý plátce, nezveřejněný účet).
 * Texty přicházejí z backendu česky; doplňujeme vysvětlení ručení.
 */
export function ExpenseWarnings({ warnings }: { warnings: readonly string[] | null | undefined }) {
  if (!warnings || warnings.length === 0) return null
  return (
    <div role="alert" className="flex gap-3 rounded-xl border border-warning/50 bg-warning/10 p-4 text-sm">
      <TriangleAlertIcon className="mt-0.5 size-5 shrink-0 text-warning-foreground dark:text-warning" />
      <div className="flex min-w-0 flex-col gap-2">
        <ul className="flex flex-col gap-1 font-medium">
          {warnings.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
        <p className="text-muted-foreground">
          Podle § 109 zákona o DPH ručíte za daň, kterou dodavatel neodvede, pokud zaplatíte nespolehlivému plátci (bez ohledu
          na částku), nebo pokud platbu nad 540 000 Kč pošlete na účet, který není zveřejněný v registru plátců DPH. Ručení se
          vyhnete, když DPH z faktury uhradíte přímo správci daně dodavatele (zvláštní způsob zajištění daně, § 109a).
        </p>
      </div>
    </div>
  )
}
