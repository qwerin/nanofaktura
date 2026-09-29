import { useMemo } from 'react'
import { encode } from 'uqr'
import { cn } from '@/lib/utils'

/**
 * QR kód jako SVG (vykresleno v prohlížeči z řetězce, např. SPAYD pro QR Platbu).
 * Vždy tmavé moduly na bílém podkladu s tichou zónou — kvůli čitelnosti i v tmavém režimu.
 */
export function QrCode({ value, label, className }: { value: string; label: string; className?: string }) {
  const { path, size } = useMemo(() => {
    // Úroveň M dle doporučení QR Platby (ČBA).
    const qr = encode(value, { ecc: 'M', border: 0 })
    let d = ''
    qr.data.forEach((row, y) =>
      row.forEach((dark, x) => {
        if (dark) d += `M${x} ${y}h1v1h-1z`
      }),
    )
    return { path: d, size: qr.size }
  }, [value])

  const quiet = 4
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`${-quiet} ${-quiet} ${size + quiet * 2} ${size + quiet * 2}`}
      shapeRendering="crispEdges"
      className={cn('block aspect-square rounded-lg bg-white', className)}
    >
      <path d={path} fill="#000" />
    </svg>
  )
}
