import { createFileRoute } from '@tanstack/react-router'
import { CircleAlertIcon, CircleCheckIcon, ClockIcon, LinkIcon } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { errorMessage, hasErrorCode, isApiError } from '@/api/errors'
import { useConfirmEmailVerification } from '@/api/queries/auth'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// Ověření e-mailové adresy z odkazu v e-mailu (SPEC §3.2).
export const Route = createFileRoute('/verify-email/$token')({
  head: () => ({ meta: [{ title: 'Ověření e-mailu · NanoFaktura' }] }),
  component: VerifyEmailPage,
})

function VerifyEmailPage() {
  const { token } = Route.useParams()
  const confirm = useConfirmEmailVerification()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    confirm.mutate(token)
  }, [confirm, token])

  const home = <ButtonLink to="/">Pokračovat do aplikace</ButtonLink>

  if (confirm.isSuccess) {
    return (
      <AuthLayout>
        <EmptyState
          icon={CircleCheckIcon}
          title="E-mail je ověřený"
          description={`Adresa ${confirm.data.email} je ověřená. Děkujeme.`}
          action={home}
        />
      </AuthLayout>
    )
  }
  if (confirm.isError) {
    const err = confirm.error
    if (isApiError(err) && err.status === 404) {
      return (
        <AuthLayout>
          <EmptyState
            icon={LinkIcon}
            title="Odkaz nenalezen"
            description="Odkaz je neúplný, neplatný, nebo jste si mezitím nechali poslat novější. Otevřete poslední odkaz z e-mailu."
            action={home}
          />
        </AuthLayout>
      )
    }
    if (hasErrorCode(err, 'verification_expired')) {
      return (
        <AuthLayout>
          <EmptyState
            icon={ClockIcon}
            title="Odkaz už neplatí"
            description="Platí 24 hodin a jde použít jednou. Nový odkaz si pošlete v aplikaci v Nastavení › Zabezpečení."
            action={home}
          />
        </AuthLayout>
      )
    }
    return (
      <AuthLayout>
        <EmptyState
          icon={CircleAlertIcon}
          title="E-mail se nepodařilo ověřit"
          description={errorMessage(err)}
          action={<Button onClick={() => confirm.mutate(token)}>Zkusit znovu</Button>}
        />
      </AuthLayout>
    )
  }
  return (
    <AuthLayout title="Ověření e-mailu">
      <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
        <Spinner /> Ověřuji…
      </div>
    </AuthLayout>
  )
}
