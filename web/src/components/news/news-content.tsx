import type { Block, Inline } from '@/lib/changelog'

function InlineText({ content }: { content: Inline[] }) {
  return content.map((part, i) => {
    switch (part.type) {
      case 'strong':
        return <strong key={i} className="font-semibold text-foreground">{part.text}</strong>
      case 'em':
        return <em key={i} className="text-foreground/90 not-italic font-medium">{part.text}</em>
      case 'code':
        return <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">{part.text}</code>
      case 'link':
        return (
          <a key={i} href={part.href} target="_blank" rel="noreferrer" className="text-primary underline underline-offset-2">
            {part.text}
          </a>
        )
      default:
        return <span key={i}>{part.text}</span>
    }
  })
}

/** Obsah jednoho záznamu novinek (jen text — žádné vkládání HTML). */
export function NewsContent({ blocks }: { blocks: Block[] }) {
  return (
    <div className="space-y-3 text-sm leading-relaxed text-muted-foreground">
      {blocks.map((b, i) =>
        b.type === 'heading' ? (
          <h3 key={i} className="pt-1 text-sm font-semibold text-foreground">
            <InlineText content={b.content} />
          </h3>
        ) : b.type === 'list' ? (
          <ul key={i} className="list-disc space-y-2 pl-5 marker:text-primary/60">
            {b.items.map((item, j) => (
              <li key={j}>
                <InlineText content={item} />
              </li>
            ))}
          </ul>
        ) : (
          <p key={i}>
            <InlineText content={b.content} />
          </p>
        ),
      )}
    </div>
  )
}
