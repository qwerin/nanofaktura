import { startFakes } from './lib/fakes'
import { buildAll, SETUP_TOKEN, startServer } from './lib/server'

// Builds once, starts the fakes and the shared instance; workers read the URLs
// from the environment (inherited from this process). Returns the teardown.
export default async function globalSetup() {
  buildAll()
  const fakes = await startFakes()
  const server = await startServer(fakes)
  // The first registration needs the setup token; afterwards signup is open
  // (NANOFAKTURA_ALLOW_SIGNUP) and every test registers its own account.
  const res = await fetch(`${server.url}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'bootstrap@e2e.test', name: 'Bootstrap', password: 'bootstrap-1234', account_name: 'Bootstrap', setup_token: SETUP_TOKEN }),
  })
  if (!res.ok) throw new Error(`bootstrap registration: ${res.status} ${await res.text()}`)
  process.env.E2E_BASE_URL = server.url
  process.env.E2E_FAKES_URL = fakes.url
  process.env.E2E_SMTP_PORT = String(fakes.smtpPort)
  process.env.E2E_SERVER_LOG = server.logFile
  return async () => {
    await server.stop()
    await fakes.close()
  }
}
