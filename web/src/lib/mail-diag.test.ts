import { describe, expect, it } from 'vitest'
import { formatDuration, formatUptime, groupChecks, reportSummary, worstStatus, type DiagStatus } from './mail-diag'

const c = (id: string, status: DiagStatus = 'ok') => ({ id, status })

describe('worstStatus', () => {
  it('picks the most severe', () => {
    expect(worstStatus([])).toBe('ok')
    expect(worstStatus(['ok', 'info'])).toBe('info')
    expect(worstStatus(['warning', 'info', 'ok'])).toBe('warning')
    expect(worstStatus(['ok', 'error', 'warning'])).toBe('error')
  })
})

describe('groupChecks', () => {
  it('groups SMTP steps and orders DNS checks', () => {
    const groups = groupChecks({
      smtp: [c('resolve'), c('connect'), c('banner'), c('ehlo'), c('starttls', 'warning'), c('auth', 'info'), c('mail_from'), c('rcpt_to', 'error')],
      dns: [c('mx'), c('spf', 'warning'), c('dmarc'), c('dkim', 'info')],
    })
    expect(groups.map((g) => g.id)).toEqual(['connection', 'tls', 'auth', 'send', 'dns'])
    expect(groups.map((g) => g.status)).toEqual(['ok', 'warning', 'info', 'error', 'warning'])
    expect(groups[4]?.checks.map((x) => x.id)).toEqual(['spf', 'dkim', 'dmarc', 'mx'])
  })

  it('drops empty groups and keeps unknown steps under Připojení', () => {
    const groups = groupChecks({ smtp: [c('config', 'error'), c('something')], dns: null })
    expect(groups).toHaveLength(1)
    expect(groups[0]).toMatchObject({ id: 'connection', label: 'Připojení', status: 'error' })
    expect(groups[0]?.checks).toHaveLength(2)
  })
})

describe('reportSummary', () => {
  it('describes the outcome', () => {
    expect(reportSummary({ status: 'error', sent: false, to: 'a@b.cz' })).toContain('nepodařilo')
    expect(reportSummary({ status: 'warning', sent: true, to: 'a@b.cz' })).toContain('nedostatky')
    expect(reportSummary({ status: 'ok', sent: true, to: 'a@b.cz' })).toContain('a@b.cz')
  })
})

describe('formatting', () => {
  it('formats uptime', () => {
    expect(formatUptime(42)).toBe('42 s')
    expect(formatUptime(95)).toBe('1 min')
    expect(formatUptime(7200)).toBe('2 h')
    expect(formatUptime(3900)).toBe('1 h 5 min')
    expect(formatUptime(93_784)).toBe('1 d 2 h')
    expect(formatUptime(172_800)).toBe('2 d')
  })
  it('formats durations', () => {
    expect(formatDuration(120)).toBe('120 ms')
    expect(formatDuration(1250)).toMatch(/^1,3\s?s$/)
  })
})
