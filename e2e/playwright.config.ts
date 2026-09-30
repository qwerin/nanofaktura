/**
 * NanoFaktura end-to-end tests (Playwright).
 *
 *   cd e2e && npm ci && npm test          # builds Go binary + SPA, runs desktop + mobile projects
 *   E2E_SKIP_BUILD=1 npm test             # reuse e2e/.build/nanofaktura and web/dist
 *   npx playwright test tests/invoice.spec.ts --project=desktop
 *
 * global-setup.ts builds the binary + SPA, starts lib/fakes.ts (ARES, ČNB, VIES, VAT registry,
 * Fio and an SMTP sink, see the header there) and one server on a random 127.0.0.1 port with a
 * temp SQLite DB (signup open, rate limits off). Every test registers its own account through
 * lib/fixtures.ts (`owner`, `api`, `mail` …), so tests are independent and run in parallel.
 * Tests needing a pristine instance start their own with `startInstance()`.
 * Every test fails on a CSP violation or an uncaught page error (auto fixture).
 * Locally the system Chrome is used; in CI (env CI) the Playwright Chromium.
 */
import { defineConfig, devices } from '@playwright/test'

const browser = process.env.CI ? {} : { channel: 'chrome' as const }

export default defineConfig({
  testDir: './tests',
  globalSetup: './global-setup.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 4 : undefined,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    locale: 'cs-CZ',
    timezoneId: 'Europe/Prague',
    acceptDownloads: true,
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },
  projects: [
    {
      name: 'desktop',
      use: { ...devices['Desktop Chrome'], ...browser, viewport: { width: 1440, height: 900 } },
    },
    {
      name: 'mobile',
      use: {
        ...devices['Pixel 7'],
        ...browser,
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
})
