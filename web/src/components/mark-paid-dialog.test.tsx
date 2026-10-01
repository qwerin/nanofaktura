import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { API, server } from '@/test/server'
import { MarkPaidDialog } from './mark-paid-dialog'

function mockMarkPaid() {
  const calls: { query: string; body: Record<string, unknown> }[] = []
  server.use(
    http.post(`${API}/api/accounts/:slug/invoices/mark-paid`, async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      calls.push({ query: new URL(request.url).search, body })
      return HttpResponse.json({
        count: 3,
        sums: [{ currency: 'CZK', count: 3, sum_total: 1_500_000, sum_remaining: 1_250_000 }],
      })
    }),
  )
  return calls
}

describe('hromadné označení jako uhrazené', () => {
  it('ukáže náhled podle filtru a odešle zvolené datum', async () => {
    const calls = mockMarkPaid()
    const onOpenChange = vi.fn()
    const { user } = renderWithProviders(
      <MarkPaidDialog
        slug="firma"
        target={{ kind: 'invoices', filters: { until: '2025-12-31', status: 'unpaid' } }}
        filtered
        open
        onOpenChange={onOpenChange}
      />,
    )
    expect(await screen.findByText('3 faktury k úhradě')).toBeInTheDocument()
    expect(calls[0]).toEqual({ query: '?until=2025-12-31&status=unpaid', body: { dry_run: true } })

    await user.click(screen.getByRole('radio', { name: 'Jedno datum pro všechny' }))
    const date = screen.getByLabelText('Datum úhrady', { selector: 'input' })
    await user.clear(date)
    await user.type(date, '2025-12-31')
    await user.click(screen.getByRole('button', { name: 'Označit 3 faktury' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(calls.filter((c) => !c.body.dry_run).map((c) => c.body)).toEqual([{ paid_on: '2025-12-31' }])
  })

  it('výchozí datum splatnosti posílá prázdné tělo', async () => {
    const calls = mockMarkPaid()
    const { user } = renderWithProviders(
      <MarkPaidDialog slug="firma" target={{ kind: 'invoices', filters: {} }} filtered={false} open onOpenChange={() => {}} />,
    )
    await user.click(await screen.findByRole('button', { name: 'Označit 3 faktury' }))
    await waitFor(() => expect(calls.filter((c) => !c.body.dry_run).map((c) => c.body)).toEqual([{}]))
  })
})
