import { useQuery } from '@tanstack/react-query'
import { MailCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { authQueries, useRequestEmailVerification } from '@/api/queries/auth'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

/**
 * Upozornění pro budoucího správce instance: e-mail je v NANOFAKTURA_ADMIN_EMAILS,
 * ale ještě není ověřený — bez ověření správcovská práva nemá.
 */
export function EmailVerifyHint() {
  const { data: me } = useQuery(authQueries.me())
  const request = useRequestEmailVerification()
  const [sent, setSent] = useState(false)
  if (!me?.instance_admin_pending) return null

  return (
    <Alert className="mb-6">
      <MailCheckIcon />
      <AlertTitle>Ověřte svůj e-mail</AlertTitle>
      <AlertDescription>
        <p>
          Adresa {me.user.email} patří správci této instance. Správu instance zpřístupníme, až potvrdíte, že vám na ni chodí
          pošta.
        </p>
        {sent ? (
          <p className="font-medium text-foreground">Odkaz jsme poslali na {me.user.email}. Platí 24 hodin.</p>
        ) : (
          <Button
            size="sm"
            className="mt-2"
            disabled={request.isPending}
            onClick={() =>
              request.mutate(undefined, {
                onSuccess: () => {
                  setSent(true)
                  toast.success('Ověřovací odkaz odeslán.')
                },
              })
            }
          >
            {request.isPending && <Spinner data-icon="inline-start" />}
            Ověřit e-mail
          </Button>
        )}
      </AlertDescription>
    </Alert>
  )
}
