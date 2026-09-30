import { useNavigate } from '@tanstack/react-router'
import type { MouseEvent, ReactNode } from 'react'

/**
 * Odkaz na hotovou SPA adresu (`url_hint` z hledání, `recordHref`) — klient-side navigace,
 * ale Ctrl/⌘/střední klik otevře novou záložku jako běžný odkaz.
 */
export function HrefLink({
  href,
  className,
  children,
  onNavigate,
  ...rest
}: {
  href: string
  className?: string
  children: ReactNode
  onNavigate?: () => void
  'aria-label'?: string
}) {
  const navigate = useNavigate()
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    onNavigate?.()
    void navigate({ href })
  }
  return (
    <a href={href} className={className} onClick={onClick} {...rest}>
      {children}
    </a>
  )
}
