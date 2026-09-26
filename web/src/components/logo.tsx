import { cn } from '@/lib/utils'

/** Značka aplikace: zaoblený čtverec s „listem faktury“. Barvy z tokenů (funguje ve světlém i tmavém režimu). */
export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true" className={cn('size-8 shrink-0', className)}>
      <rect width="32" height="32" rx="8" className="fill-primary" />
      <g transform="translate(16 16) scale(1.3) translate(-16 -16)">
        <path
          d="M11 8.5h7.2L22 12.3V22a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 10 22V10a1.5 1.5 0 0 1 1-1.5Z"
          className="fill-primary-foreground"
        />
        <path d="M13 15h6M13 18h6M13 12h3" strokeWidth="1.6" strokeLinecap="round" className="stroke-primary" />
      </g>
    </svg>
  )
}

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2', className)}>
      <LogoMark />
      <span className="text-base font-semibold tracking-tight">NanoFaktura</span>
    </span>
  )
}
