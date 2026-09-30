import { createContext, useContext } from 'react'

export interface CommandPaletteApi {
  /** Otevře paletu (volitelně s předvyplněným dotazem). */
  open: (query?: string) => void
  /** Dialog s přehledem klávesových zkratek. */
  openHelp: () => void
}

export const CommandPaletteContext = createContext<CommandPaletteApi | null>(null)

/** `null` mimo AppShell (např. průvodce onboardingem) — tlačítka hledání se pak nezobrazí. */
export function useCommandPalette(): CommandPaletteApi | null {
  return useContext(CommandPaletteContext)
}
