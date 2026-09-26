import { MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'
import { createContext, useContext } from 'react'

export type ThemeChoice = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

/** Klíč v localStorage — čte ho i inline skript v index.html (zamezí probliknutí). */
export const THEME_STORAGE_KEY = 'nf-theme'

export const themeOptions: { value: ThemeChoice; label: string; icon: typeof SunIcon }[] = [
  { value: 'light', label: 'Světlý', icon: SunIcon },
  { value: 'dark', label: 'Tmavý', icon: MoonIcon },
  { value: 'system', label: 'Systém', icon: MonitorIcon },
]

export interface ThemeContextValue {
  theme: ThemeChoice
  resolvedTheme: ResolvedTheme
  setTheme: (t: ThemeChoice) => void
}

export const ThemeContext = createContext<ThemeContextValue>({
  theme: 'system',
  resolvedTheme: 'light',
  setTheme: () => {},
})

/** Zvolený motiv (výchozí „system“) a skutečně použitý světlý/tmavý režim. */
export function useThemeChoice(): ThemeContextValue {
  return useContext(ThemeContext)
}
