// Builds and starts NanoFaktura instances for the E2E suite. The main instance is
// started once by global-setup.ts; tests that need a pristine instance (first
// registration, rate limits) start their own with startServer().
import { execFileSync, spawn, type ChildProcess } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'

export const repoRoot = path.resolve(import.meta.dirname, '..', '..')
export const buildDir = path.join(repoRoot, 'e2e', '.build')
export const binary = path.join(buildDir, 'nanofaktura')
export const staticDir = path.join(repoRoot, 'web', 'dist')

/** Builds the Go binary and the SPA (skipped with E2E_SKIP_BUILD=1 when both exist). */
export function buildAll(): void {
  const have = fs.existsSync(binary) && fs.existsSync(path.join(staticDir, 'index.html'))
  if (process.env.E2E_SKIP_BUILD === '1' && have) return
  fs.mkdirSync(buildDir, { recursive: true })
  execFileSync('go', ['build', '-o', binary, './cmd/server'], {
    cwd: repoRoot,
    env: { ...process.env, CGO_ENABLED: '0' },
    stdio: 'inherit',
  })
  execFileSync('npm', ['run', 'build'], { cwd: path.join(repoRoot, 'web'), stdio: 'inherit' })
}

export function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.unref()
    srv.on('error', reject)
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address() as net.AddressInfo
      srv.close(() => resolve(port))
    })
  })
}

export interface Fakes {
  /** Base URL of the fake external services (ARES, ČNB, VIES, VAT registry, Fio, mail inbox). */
  url: string
  smtpPort: number
}

export interface Instance {
  url: string
  dataDir: string
  logFile: string
  stop(): Promise<void>
}

export interface ServerOptions {
  env?: Record<string, string>
  /** Default true: registrations always allowed (every test creates its own account). */
  allowSignup?: boolean
  /** Default true: rate limits off so parallel tests from 127.0.0.1 do not interfere. */
  disableRateLimit?: boolean
}

export const SETUP_TOKEN = 'e2e-setup-token'
/** Instance admins (one per Playwright project, so both can run the admin flow). */
export const ADMIN_EMAILS = ['admin-desktop@e2e.test', 'admin-mobile@e2e.test']

export async function startServer(fakes: Fakes, opts: ServerOptions = {}): Promise<Instance> {
  const port = await freePort()
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'nanofaktura-e2e-'))
  const url = `http://127.0.0.1:${port}`
  const logFile = path.join(dataDir, 'server.log')
  const log = fs.openSync(logFile, 'a')
  const env: Record<string, string> = {
    PATH: process.env.PATH ?? '',
    HOME: process.env.HOME ?? dataDir,
    TZ: 'Europe/Prague',
    NANOFAKTURA_LISTEN_ADDR: `127.0.0.1:${port}`,
    NANOFAKTURA_DB_DRIVER: 'sqlite',
    NANOFAKTURA_DB_DSN: path.join(dataDir, 'nanofaktura.db'),
    NANOFAKTURA_DATA_DIR: dataDir,
    NANOFAKTURA_STATIC_DIR: staticDir,
    NANOFAKTURA_PUBLIC_URL: url,
    NANOFAKTURA_ALLOW_SIGNUP: String(opts.allowSignup ?? true),
    NANOFAKTURA_DISABLE_RATE_LIMIT: String(opts.disableRateLimit ?? true),
    NANOFAKTURA_SETUP_TOKEN: SETUP_TOKEN,
    NANOFAKTURA_ADMIN_EMAILS: ADMIN_EMAILS.join(','),
    NANOFAKTURA_SMTP_HOST: '127.0.0.1',
    NANOFAKTURA_SMTP_PORT: String(fakes.smtpPort),
    NANOFAKTURA_SMTP_TLS: 'none',
    NANOFAKTURA_MAIL_FROM: 'NanoFaktura <faktury@e2e.test>',
    NANOFAKTURA_ARES_URL: `${fakes.url}/ares`,
    NANOFAKTURA_CNB_URL: `${fakes.url}/cnb/denni_kurz.txt`,
    NANOFAKTURA_VIES_URL: `${fakes.url}/vies`,
    NANOFAKTURA_VATREG_URL: `${fakes.url}/vatreg`,
    NANOFAKTURA_FIO_URL: `${fakes.url}/fio`,
    NANOFAKTURA_DB_LOG: 'silent',
    ...opts.env,
  }
  const child: ChildProcess = spawn(binary, [], { env, stdio: ['ignore', log, log] })
  let exited = false
  child.on('exit', () => (exited = true))
  const deadline = Date.now() + 20_000
  for (;;) {
    if (exited) throw new Error(`server exited early, see ${logFile}:\n${fs.readFileSync(logFile, 'utf8')}`)
    try {
      const res = await fetch(`${url}/api/health`)
      if (res.ok) break
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) throw new Error(`server did not start, see ${logFile}`)
    await new Promise((r) => setTimeout(r, 100))
  }
  return {
    url,
    dataDir,
    logFile,
    async stop() {
      if (!exited) {
        const done = new Promise((r) => child.once('exit', r))
        child.kill('SIGTERM')
        await Promise.race([done, new Promise((r) => setTimeout(r, 5000))])
        if (!exited) child.kill('SIGKILL')
      }
      fs.closeSync(log)
      if (!process.env.E2E_KEEP_DATA) fs.rmSync(dataDir, { recursive: true, force: true })
    },
  }
}
