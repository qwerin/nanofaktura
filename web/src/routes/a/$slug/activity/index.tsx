import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { eventFamilyFilters } from '@/components/events/event-meta'
import { Timeline } from '@/components/events/timeline'
import { PageBody, PageHeader } from '@/components/page-header'
import { cn } from '@/lib/utils'

const families = eventFamilyFilters.map((f) => f.family) as [string, ...string[]]

const searchSchema = z.object({
  family: z.enum(families).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/activity/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Aktivita · NanoFaktura' }] }),
  component: ActivityPage,
})

function ActivityPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const family = eventFamilyFilters.find((f) => f.family === search.family)
  // API filtruje jedním prefixem; rodina „Náklady“ má dva (expense, expense_payment) → první stačí pro hlavní události.
  const filters = family ? { name: family.prefixes[0] } : {}

  return (
    <>
      <PageHeader
        title="Aktivita"
        description="Co se v účtu dělo — kdo co vystavil, odeslal, zaplatil nebo změnil."
        back={{ to: '/a/$slug/dashboard', params: { slug } }}
      >
        <div role="group" aria-label="Typ události" className="-mx-4 flex gap-2 overflow-x-auto px-4 pt-1 pb-1 [scrollbar-width:none] md:mx-0 md:flex-wrap md:px-0">
          <Chip active={!family} onClick={() => void navigate({ search: {}, replace: true })}>
            Vše
          </Chip>
          {eventFamilyFilters.map((f) => (
            <Chip key={f.family} active={family?.family === f.family} onClick={() => void navigate({ search: { family: f.family }, replace: true })}>
              {f.label}
            </Chip>
          ))}
        </div>
      </PageHeader>
      <PageBody>
        <div className="max-w-3xl rounded-xl border bg-card p-4 md:p-5">
          <Timeline key={family?.family ?? 'all'} filters={filters} perPage={30} linkRecords />
        </div>
      </PageBody>
    </>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'h-11 shrink-0 rounded-full border px-3.5 text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:h-7 md:px-3 md:text-xs',
        active ? 'border-primary bg-primary text-primary-foreground' : 'bg-background text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}
