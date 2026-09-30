// Zod bez JIT (new Function): přísná CSP (script-src 'self', bez 'unsafe-eval') by
// jinak hlásila porušení. Importuje se v main.tsx jako první, před jakýmkoli schématem.
import { z } from 'zod'

z.config({ jitless: true })
