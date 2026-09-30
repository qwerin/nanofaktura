import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { accountQueries } from '@/api/queries/accounts'
import { BrandImageField } from '@/components/branding/brand-image-field'
import { PdfPreview } from '@/components/branding/pdf-preview'
import { PageError } from '@/components/page-states'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useCurrentAccount } from '@/hooks/use-current-account'

export const Route = createFileRoute('/a/$slug/settings/appearance')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'Vzhled dokladů · NanoFaktura' }] }),
  component: AppearancePage,
})

function AppearancePage() {
  const { slug } = Route.useParams()
  const { canManageSettings } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug))

  return (
    <SettingsPage
      title="Vzhled dokladů"
      description="Logo a podpis se tisknou na každé PDF faktury — i na těch, které už jste vystavili."
    >
      {!canManageSettings && (
        <Alert className="mb-6 max-w-2xl">
          <LockIcon />
          <AlertDescription>Vzhled dokladů může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}
      {account.isError ? (
        <PageError error={account.error} reset={() => void account.refetch()} />
      ) : account.isPending ? (
        <FormSkeleton fields={3} />
      ) : (
        <div className="max-w-4xl">
          <FormSection title="Logo" description="Zobrazí se v záhlaví faktury vedle údajů dodavatele.">
            <BrandImageField
              slug={slug}
              kind="logo"
              attachmentId={account.data.logo_attachment_id || undefined}
              canEdit={canManageSettings}
            />
          </FormSection>
          <FormSection title="Podpis a razítko" description="Vloží se dole na fakturu, nad podpisovou čáru.">
            <BrandImageField
              slug={slug}
              kind="stamp"
              attachmentId={account.data.stamp_attachment_id || undefined}
              canEdit={canManageSettings}
            />
          </FormSection>
          <FormSection
            title="Šablona a náhled"
            description="Šablona, barva, QR platba a patička všech PDF dokladů. Náhled se mění hned, uloží se tlačítkem."
          >
            <PdfPreview slug={slug} account={account.data} canEdit={canManageSettings} />
          </FormSection>
        </div>
      )}
    </SettingsPage>
  )
}
