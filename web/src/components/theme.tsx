import { useCallback, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react'
import {
  THEME_STORAGE_KEY,
  ThemeContext,
  themeOptions,
  useThemeChoice,
  type ThemeChoice,
} from '@/hooks/use-theme-choice'
import { cn } from '@/lib/utils'

// Barvy pro <meta name="theme-color"> (odpovídají --background v index.css).
const THEME_COLORS = { light: '#fbfbfd', dark: '#111217' } as const
const DARK_QUERY = '(prefers-color-scheme: dark)'

function readStoredTheme(): ThemeChoice {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY)
    return v === 'light' || v === 'dark' ? v : 'system'
  } catch {
    return 'system'
  }
}

function subscribeSystem(onChange: () => void) {
  const mql = window.matchMedia(DARK_QUERY)
  mql.addEventListener('change', onChange)
  return () => mql.removeEventListener('change', onChange)
}

/**
 * Světlý/tmavý režim: třída `.dark` na <html>, volba v localStorage (`nf-theme`), výchozí dle systému.
 * Počáteční třídu nastaví už skript public/theme-init.js (načtený z index.html), tady se jen udržuje v synchronizaci.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeChoice>(readStoredTheme)
  const systemDark = useSyncExternalStore(
    subscribeSystem,
    () => window.matchMedia(DARK_QUERY).matches,
    () => false,
  )
  const resolvedTheme = theme === 'system' ? (systemDark ? 'dark' : 'light') : theme

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle('dark', resolvedTheme === 'dark')
    root.style.colorScheme = resolvedTheme
    document.querySelectorAll('meta[name="theme-color"]').forEach((m) => {
      m.setAttribute('content', THEME_COLORS[resolvedTheme])
    })
  }, [resolvedTheme])

  const setTheme = useCallback((t: ThemeChoice) => {
    setThemeState(t)
    try {
      if (t === 'system') localStorage.removeItem(THEME_STORAGE_KEY)
      else localStorage.setItem(THEME_STORAGE_KEY, t)
    } catch {
      // localStorage nedostupný (privátní režim) — volba platí jen do reloadu
    }
  }, [])

  const value = useMemo(() => ({ theme, resolvedTheme, setTheme }), [theme, resolvedTheme, setTheme])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

/** Segmentový přepínač motivu (Světlý / Tmavý / Systém). */
export function ThemeSegmented({ className }: { className?: string }) {
  const { theme, setTheme } = useThemeChoice()
  return (
    <div
      role="radiogroup"
      aria-label="Motiv"
      className={cn('grid grid-cols-3 gap-1 rounded-lg bg-muted p-1', className)}
    >
      {themeOptions.map(({ value, label, icon: Icon }) => (
        <button
          key={value}
          type="button"
          role="radio"
          aria-checked={theme === value}
          onClick={() => setTheme(value)}
          className={cn(
            'flex h-10 items-center justify-center gap-1.5 rounded-md text-sm font-medium text-muted-foreground transition-colors md:h-8',
            'hover:text-foreground aria-checked:bg-background aria-checked:text-foreground aria-checked:shadow-sm',
          )}
        >
          <Icon className="size-4" />
          {label}
        </button>
      ))}
    </div>
  )
}
