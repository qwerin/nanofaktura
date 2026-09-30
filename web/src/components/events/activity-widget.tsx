import { Link } from '@tanstack/react-router'
import { ActivityIcon } from 'lucide-react'
import { Timeline } from './timeline'

const ALL = {} as const

/** Přehled: posledních 8 událostí účtu s odkazy na záznamy. */
export function ActivityWidget({ slug }: { slug: string }) {
  return (
    <section className="flex flex-col rounded-xl border bg-card">
      <div className="flex items-center justify-between gap-2 px-4 pt-4 pb-3 md:px-5">
        <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
          <ActivityIcon className="size-4 text-primary" aria-hidden="true" />
          Poslední aktivita
        </h2>
        <Link to="/a/$slug/activity" params={{ slug }} className="text-sm font-medium text-primary underline-offset-4 hover:underline">
          Zobrazit vše
        </Link>
      </div>
      <Timeline filters={ALL} perPage={8} linkRecords noPaging className="px-4 pb-4 md:px-5 md:pb-5" />
    </section>
  )
}
