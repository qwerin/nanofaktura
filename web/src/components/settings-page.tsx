import { useParams } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { cn } from '@/lib/utils'

/**
 * Obal podstránky nastavení. Na mobilu vlastní hlavička se šipkou zpět na seznam sekcí,
 * na desktopu hlavičku „Nastavení“ se záložkami kreslí layout (routes/a/$slug/settings/route.tsx).
 */
export function SettingsPage({
  title,
  description,
  actions,
  children,
  className,
}: {
  title: string
  description?: ReactNode
  actions?: PageAction[]
  children: ReactNode
  className?: string
}) {
  const { slug } = useParams({ from: '/a/$slug' })
  return (
    <>
      <PageHeader
        title={title}
        show="mobile"
        back={{ to: '/a/$slug/settings', params: { slug } }}
        actions={actions}
      />
      <PageBody className={cn('md:mx-0 md:max-w-none md:px-0 md:pt-0', className)}>
        {description && <p className="mb-6 max-w-2xl text-sm text-muted-foreground">{description}</p>}
        {children}
      </PageBody>
    </>
  )
}

/** Skupina polí formuláře: nadpis + popis vlevo (desktop), pole vpravo; na mobilu pod sebou. */
export function FormSection({
  title,
  description,
  children,
}: {
  title: string
  description?: ReactNode
  children: ReactNode
}) {
  return (
    // Dva sloupce podle skutečné šířky obsahu (container query), ne okna — vedle menu nastavení je obsah užší.
    <section className="@container border-b py-6 first:pt-0 last:border-b-0 md:py-8">
      <div className="grid gap-4 @3xl:grid-cols-[minmax(0,14rem)_minmax(0,1fr)] @3xl:gap-10">
        <div>
          <h2 className="text-base font-semibold tracking-tight">{title}</h2>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        <div className="flex min-w-0 flex-col gap-5">{children}</div>
      </div>
    </section>
  )
}
