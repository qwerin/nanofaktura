import { FileTextIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { errorMessage, hasErrorCode } from '@/api/errors'
import { useDeleteSubject } from '@/api/queries/subjects'
import type { Subject } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

/** Potvrzení smazání kontaktu. Kontakt s fakturami smazat nejde (409) — vysvětlíme proč. */
export function DeleteSubjectDialog({
  slug,
  subject,
  open,
  onOpenChange,
  onDeleted,
}: {
  slug: string
  subject: Pick<Subject, 'id' | 'name'>
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: () => void
}) {
  const del = useDeleteSubject(slug)
  const [hasInvoices, setHasInvoices] = useState(false)

  const close = (next: boolean) => {
    onOpenChange(next)
    if (!next) setTimeout(() => setHasInvoices(false), 300)
  }

  if (hasInvoices) {
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={close}
        title="Kontakt nelze smazat"
        footer={<Button onClick={() => close(false)}>Rozumím</Button>}
      >
        <Alert>
          <FileTextIcon />
          <AlertTitle>{subject.name} má vystavené doklady</AlertTitle>
          <AlertDescription>
            Faktury si údaje odběratele pamatují, ale kontakt potřebujeme kvůli filtrům a přehledům. Pokud ho už
            nepotřebujete, můžete ho jen přejmenovat nebo doplnit poznámku.
          </AlertDescription>
        </Alert>
      </ResponsiveDialog>
    )
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={close}
      title="Smazat kontakt?"
      description={
        <>
          Kontakt <strong className="text-foreground">{subject.name}</strong> bude trvale odstraněn.
        </>
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              del.mutate(subject, {
                onSuccess: () => {
                  toast.success('Kontakt smazán')
                  close(false)
                  onDeleted()
                },
                onError: (err) => {
                  if (hasErrorCode(err, 'has_invoices')) setHasInvoices(true)
                  else toast.error(errorMessage(err))
                },
              })
            }
          >
            {del.isPending && <Spinner data-icon="inline-start" />}
            Smazat kontakt
          </Button>
          <Button variant="outline" onClick={() => close(false)}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
