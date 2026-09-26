import type { LinkOptions } from '@tanstack/react-router'
import { ConstructionIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'

/** Zástupná stránka pro obrazovky, které ještě nejsou hotové. */
export function ComingSoon({
  title,
  description,
  back,
  actions,
  children,
}: {
  title: string
  description?: string
  back?: LinkOptions
  actions?: PageAction[]
  children?: ReactNode
}) {
  return (
    <>
      <PageHeader title={title} description={description} back={back} actions={actions} />
      <PageBody>
        {children}
        <EmptyState
          icon={ConstructionIcon}
          title="Připravujeme"
          description="Tato část aplikace se právě staví."
        />
      </PageBody>
    </>
  )
}
