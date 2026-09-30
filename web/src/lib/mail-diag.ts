// Pomocné funkce pro výsledek testu e-mailu (Správa instance → Test e-mailu).

export type DiagStatus = 'ok' | 'info' | 'warning' | 'error'

export interface DiagCheckLike {
  id: string
  status: DiagStatus
}

export interface DiagGroup<C extends DiagCheckLike> {
  id: string
  label: string
  status: DiagStatus
  checks: C[]
}

const smtpGroups: { id: string; label: string; ids: string[] }[] = [
  { id: 'connection', label: 'Připojení', ids: ['config', 'resolve', 'connect', 'banner', 'ehlo'] },
  { id: 'tls', label: 'TLS', ids: ['tls', 'starttls', 'ehlo_tls'] },
  { id: 'auth', label: 'Přihlášení', ids: ['auth'] },
  { id: 'send', label: 'Odeslání', ids: ['mail_from', 'rcpt_to', 'data'] },
]

/** Pořadí DNS kontrol: SPF · DKIM · DMARC · MX. */
const dnsOrder = ['spf', 'dkim', 'dmarc', 'mx']

const rank: Record<DiagStatus, number> = { ok: 0, info: 1, warning: 2, error: 3 }

/** Nejhorší stav (ok < info < warning < error); prázdný seznam = ok. */
export function worstStatus(statuses: DiagStatus[]): DiagStatus {
  return statuses.reduce<DiagStatus>((w, s) => (rank[s] > rank[w] ? s : w), 'ok')
}

/**
 * Rozdělí kroky testu do skupin Připojení / TLS / Přihlášení / Odeslání / DNS.
 * Neznámé SMTP kroky patří do Připojení; prázdné skupiny se vynechají.
 */
export function groupChecks<C extends DiagCheckLike>(report: { smtp: C[] | null; dns: C[] | null }): DiagGroup<C>[] {
  const smtp = report.smtp ?? []
  const known = new Set(smtpGroups.flatMap((g) => g.ids))
  const groups: DiagGroup<C>[] = smtpGroups.map((g) => {
    const checks = smtp.filter((c) => g.ids.includes(c.id) || (g.id === 'connection' && !known.has(c.id)))
    return { id: g.id, label: g.label, status: worstStatus(checks.map((c) => c.status)), checks }
  })
  const dns = [...(report.dns ?? [])].sort((a, b) => order(a.id) - order(b.id))
  groups.push({ id: 'dns', label: 'DNS: SPF · DKIM · DMARC · MX', status: worstStatus(dns.map((c) => c.status)), checks: dns })
  return groups.filter((g) => g.checks.length > 0)
}

function order(id: string): number {
  const i = dnsOrder.indexOf(id)
  return i < 0 ? dnsOrder.length : i
}

export const statusLabels: Record<DiagStatus, string> = {
  ok: 'V pořádku',
  info: 'Informace',
  warning: 'Varování',
  error: 'Chyba',
}

/** Shrnutí celého testu jednou větou. */
export function reportSummary(r: { status: DiagStatus; sent: boolean; to: string }): string {
  if (!r.sent) return 'Testovací e-mail se nepodařilo odeslat — podívejte se na kroky označené jako chyba.'
  if (r.status === 'error' || r.status === 'warning') {
    return `Testovací e-mail pro ${r.to} server převzal, ale nastavení má nedostatky — e-maily mohou končit ve spamu.`
  }
  return `Testovací e-mail pro ${r.to} server převzal. Zkontrolujte schránku (i spam) a v Gmailu „Zobrazit originál“.`
}

/** 95 → „1 min“, 7 200 → „2 h“, 93 784 → „1 d 2 h“. */
export function formatUptime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const d = Math.floor(s / 86_400)
  const h = Math.floor((s % 86_400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return h > 0 ? `${d} d ${h} h` : `${d} d`
  if (h > 0) return m > 0 ? `${h} h ${m} min` : `${h} h`
  if (m > 0) return `${m} min`
  return `${s} s`
}

/** 120 → „120 ms“, 1 250 → „1,3 s“. */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.max(0, Math.round(ms))} ms`
  return `${(ms / 1000).toLocaleString('cs-CZ', { maximumFractionDigits: 1 })} s`
}
