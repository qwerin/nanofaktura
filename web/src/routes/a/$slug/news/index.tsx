import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { NewsContent } from '@/components/news/news-content'
import { PageBody, PageHeader } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useNews } from '@/hooks/use-news'
import { formatDateLong } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/news/')({
  head: () => ({ meta: [{ title: 'Novinky · NanoFaktura' }] }),
  component: NewsPage,
})

function NewsPage() {
  const { slug } = Route.useParams()
  const { entries, lastSeen, markSeen } = useNews()
  // co bylo nové při otevření stránky — po označení přečteného se zvýraznění nesmí ztratit
  const [seenBefore] = useState(lastSeen)

  useEffect(() => markSeen(), [markSeen])

  return (
    <>
      <PageHeader
        title="Novinky"
        description="Co přibylo a co jsme vylepšili."
        back={{ to: '/a/$slug/dashboard', params: { slug } }}
      />
      <PageBody className="max-w-3xl">
        <ol className="space-y-4">
          {entries.map((e) => {
            const isNew = !seenBefore || e.date > seenBefore
            return (
              <li key={e.date}>
                <Card className={isNew ? 'ring-1 ring-primary/30' : undefined}>
                  <CardHeader className="gap-1">
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <time dateTime={e.date}>{formatDateLong(e.date)}</time>
                      {isNew && <Badge>Nové</Badge>}
                    </div>
                    <CardTitle className="text-base md:text-lg">{e.title}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <NewsContent blocks={e.blocks} />
                  </CardContent>
                </Card>
              </li>
            )
          })}
        </ol>
      </PageBody>
    </>
  )
}
