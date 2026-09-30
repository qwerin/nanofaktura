// Fake external services for the E2E suite, all on one local HTTP server
// (path prefix per service) plus an SMTP sink. Responses are deterministic
// functions of the request, so tests need no setup, except Fio statements
// (POST /fio/_set/{token}) and the captured mail (GET /mail).
//
//   /ares/{ico}                         ARES REST: "Firma {ico} s.r.o.", Praha; IČO starting with 9 → 404
//   /cnb/denni_kurz.txt?date=…          ČNB daily list (EUR 24,350 / USD 21,500 / 100 JPY 14,000)
//   /vies/ms/{cc}/vat/{num}             VIES: valid, number starting with 0 → invalid
//   /vatreg  (SOAP)                     VAT registry: DIČ starting with 6 → unreliable payer (ANO),
//                                       starting with 9 → NENALEZEN, otherwise reliable (NE)
//   /fio/(last|periods|set-last-date)…  Fio API; statement per token (default: empty)
//   /mail?to=…                          captured e-mails (JSON), DELETE /mail clears
import http from 'node:http'
import type net from 'node:net'
import { simpleParser } from 'mailparser'
import { SMTPServer } from 'smtp-server'

export interface CapturedMail {
  from: string
  to: string[]
  subject: string
  text: string
  html: string
  attachments: { filename: string; contentType: string; size: number }[]
  receivedAt: string
}

const pad = (n: number) => String(n).padStart(2, '0')

function aresSubject(ico: string) {
  return {
    ico,
    obchodniJmeno: `Firma ${ico} s.r.o.`,
    dic: `CZ${ico}`,
    sidlo: {
      kodStatu: 'CZ',
      nazevObce: 'Praha',
      nazevCastiObce: 'Nové Město',
      nazevUlice: 'Testovací',
      cisloDomovni: 1234,
      cisloOrientacni: 5,
      psc: 11000,
    },
  }
}

function cnbList(): string {
  const d = new Date()
  return [
    `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()} #${100}`,
    'země|měna|množství|kód|kurz',
    'EMU|euro|1|EUR|24,350',
    'USA|dolar|1|USD|21,500',
    'Japonsko|jen|100|JPY|14,000',
    '',
  ].join('\n')
}

function vatregResponse(dic: string): string {
  let payer: string
  if (dic.startsWith('9')) {
    payer = `<statusPlatceDPH dic="${dic}" nespolehlivyPlatce="NENALEZEN"/>`
  } else {
    const unreliable = dic.startsWith('6')
    payer =
      `<statusPlatceDPH dic="${dic}" nespolehlivyPlatce="${unreliable ? 'ANO' : 'NE'}"` +
      (unreliable ? ' datumZverejneniNespolehlivosti="2024-03-01"' : '') +
      ` cisloFu="461"><zverejneneUcty><ucet datumZverejneni="2019-05-02">` +
      `<standardniUcet cislo="123456789" kodBanky="0100"/></ucet></zverejneneUcty>` +
      `<nazevSubjektu>Firma ${dic} s.r.o.</nazevSubjektu><adresa><uliceCislo>Testovací 1234/5</uliceCislo>` +
      `<mesto>Praha</mesto><psc>11000</psc><stat>Česká republika</stat></adresa></statusPlatceDPH>`
  }
  return (
    `<?xml version="1.0" encoding="utf-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">` +
    `<soapenv:Header/><soapenv:Body><StatusNespolehlivyPlatceRozsirenyResponse xmlns="http://adis.mfcr.cz/rozhraniCRPDPH/">` +
    `<status odpovedGenerovana="2026-09-26" statusCode="0" statusText="OK"/>${payer}` +
    `</StatusNespolehlivyPlatceRozsirenyResponse></soapenv:Body></soapenv:Envelope>`
  )
}

function emptyFio() {
  return {
    accountStatement: {
      info: { accountId: '2000000000', bankId: '2010', currency: 'CZK', iban: 'CZ7920100000002000000000', openingBalance: 0, closingBalance: 0 },
      transactionList: { transaction: [] },
    },
  }
}

function readBody(req: http.IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = []
    req.on('data', (c: Buffer) => chunks.push(c))
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')))
    req.on('error', reject)
  })
}

export interface RunningFakes {
  url: string
  smtpPort: number
  close(): Promise<void>
}

export async function startFakes(): Promise<RunningFakes> {
  const mails: CapturedMail[] = []
  const fio = new Map<string, unknown>()

  const smtp = new SMTPServer({
    authOptional: true,
    disabledCommands: ['STARTTLS', 'AUTH'],
    logger: false,
    onData(stream, session, callback) {
      simpleParser(stream)
        .then((m) => {
          mails.push({
            from: session.envelope.mailFrom ? session.envelope.mailFrom.address : '',
            to: session.envelope.rcptTo.map((r) => r.address.toLowerCase()),
            subject: m.subject ?? '',
            text: m.text ?? '',
            html: typeof m.html === 'string' ? m.html : '',
            attachments: m.attachments.map((a) => ({ filename: a.filename ?? '', contentType: a.contentType, size: a.size })),
            receivedAt: new Date().toISOString(),
          })
          callback()
        })
        .catch((err: Error) => callback(err))
    },
  })
  await new Promise<void>((resolve) => smtp.listen(0, '127.0.0.1', resolve))
  const smtpPort = ((smtp as unknown as { server: net.Server }).server.address() as net.AddressInfo).port

  const server = http.createServer(async (req, res) => {
    const u = new URL(req.url ?? '/', 'http://x')
    const p = u.pathname
    const json = (status: number, body: unknown) => {
      res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' })
      res.end(JSON.stringify(body))
    }
    let m: RegExpMatchArray | null
    try {
      if ((m = p.match(/^\/ares\/(\d{8})$/))) {
        if (m[1].startsWith('9')) return json(404, { kod: 'NENALEZENO' })
        return json(200, aresSubject(m[1]))
      }
      if (p === '/cnb/denni_kurz.txt') {
        res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' })
        return res.end(cnbList())
      }
      if ((m = p.match(/^\/vies\/ms\/([A-Z]{2})\/vat\/([^/]+)$/))) {
        const valid = !m[2].startsWith('0')
        return json(200, {
          isValid: valid,
          userError: valid ? 'VALID' : 'INVALID',
          name: valid ? `EU Company ${m[2]}` : '---',
          address: valid ? 'Hauptstraße 1\n10115 Berlin' : '---',
          vatNumber: m[2],
        })
      }
      if (p === '/vatreg' && req.method === 'POST') {
        const dic = (await readBody(req)).match(/<roz:dic>(\d+)<\/roz:dic>/)?.[1]
        if (!dic) return json(400, { error: 'no dic' })
        res.writeHead(200, { 'Content-Type': 'text/xml;charset=utf-8' })
        return res.end(vatregResponse(dic))
      }
      if ((m = p.match(/^\/fio\/_set\/([A-Za-z0-9]+)$/)) && req.method === 'POST') {
        fio.set(m[1], JSON.parse(await readBody(req)))
        return json(200, { ok: true })
      }
      if ((m = p.match(/^\/fio\/(?:last|periods)\/([A-Za-z0-9]+)\//))) {
        const st = fio.get(m[1]) ?? emptyFio()
        fio.delete(m[1]) // "last" semantics: a second download returns nothing new
        return json(200, st)
      }
      if (p.startsWith('/fio/set-last-date/')) return json(200, {})
      if (p === '/mail') {
        if (req.method === 'DELETE') {
          mails.length = 0
          return json(200, {})
        }
        const to = u.searchParams.get('to')?.toLowerCase()
        return json(200, to ? mails.filter((x) => x.to.includes(to)) : mails)
      }
      json(404, { error: `fake: no route for ${req.method} ${p}` })
    } catch (err) {
      json(500, { error: String(err) })
    }
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = (server.address() as net.AddressInfo).port
  return {
    url: `http://127.0.0.1:${port}`,
    smtpPort,
    async close() {
      await new Promise((r) => server.close(r))
      await new Promise((r) => smtp.close(() => r(undefined)))
    },
  }
}
