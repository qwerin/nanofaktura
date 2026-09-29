import { Link } from '@tanstack/react-router'
import { FileStackIcon, RepeatIcon } from 'lucide-react'

/** Přepínač „Pravidelné faktury | Šablony“ pod hlavičkou obou seznamů. */
export function RecurringTabs({ slug }: { slug: string }) {
  const cls =
    'flex h-10 flex-1 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground md:h-8 md:flex-none data-[status=active]:bg-background data-[status=active]:text-foreground data-[status=active]:shadow-sm dark:data-[status=active]:bg-input/40'
  return (
    <nav aria-label="Pravidelné faktury a šablony" className="flex gap-1 rounded-lg bg-muted p-1 md:inline-flex">
      <Link to="/a/$slug/recurring" params={{ slug }} className={cls}>
        <RepeatIcon className="size-4" />
        Pravidelné
      </Link>
      <Link to="/a/$slug/templates" params={{ slug }} className={cls}>
        <FileStackIcon className="size-4" />
        Šablony
      </Link>
    </nav>
  )
}
