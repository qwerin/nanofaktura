import { DownloadIcon, TriangleAlertIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { CopyButton } from '@/components/copy-button'
import { saveBlob } from '@/components/export/download'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { recoveryCodesText } from '@/lib/two-factor'

/**
 * Jednorázové zobrazení záložních kódů. Zavřít jde až po potvrzení, že si je uživatel uložil —
 * znovu je už nezobrazíme.
 */
export function RecoveryCodesDialog({
  codes,
  email,
  onClose,
}: {
  codes: string[] | null
  email?: string
  onClose: () => void
}) {
  const [saved, setSaved] = useState(false)
  const checkId = useId()
  const text = codes ? recoveryCodesText(codes, email) : ''

  const close = () => {
    onClose()
    setTimeout(() => setSaved(false), 300)
  }

  return (
    <ResponsiveDialog
      open={codes !== null}
      onOpenChange={(open) => {
        if (!open && saved) close()
      }}
      title="Záložní kódy"
      description="Když ztratíte telefon nebo bezpečnostní klíč, přihlásíte se jedním z těchto kódů. Každý platí jen jednou."
      footer={
        <Button onClick={close} disabled={!saved}>
          Hotovo
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <Alert>
          <TriangleAlertIcon />
          <AlertTitle>Uložte si je hned</AlertTitle>
          <AlertDescription>
            Znovu je už nezobrazíme. Stáhněte si je nebo vytiskněte a mějte je jinde než telefon.
          </AlertDescription>
        </Alert>
        <ul
          aria-label="Záložní kódy"
          className="grid grid-cols-2 gap-x-4 gap-y-2 rounded-lg border bg-muted px-4 py-3 font-mono text-sm select-all"
        >
          {codes?.map((c) => (
            <li key={c} className="tabular-nums">
              {c}
            </li>
          ))}
        </ul>
        <div className="grid gap-2 sm:grid-cols-2">
          <CopyButton value={text} label="Zkopírovat" className="w-full" />
          <Button
            variant="outline"
            className="w-full"
            onClick={() => saveBlob(new Blob([text], { type: 'text/plain;charset=utf-8' }), 'nanofaktura-zalozni-kody.txt')}
          >
            <DownloadIcon data-icon="inline-start" />
            Stáhnout .txt
          </Button>
        </div>
        <div className="flex min-h-11 items-center gap-3">
          <Checkbox id={checkId} checked={saved} onCheckedChange={(v) => setSaved(v === true)} />
          <Label htmlFor={checkId} className="text-sm font-normal">
            Kódy mám bezpečně uložené
          </Label>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
