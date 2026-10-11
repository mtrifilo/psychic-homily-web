import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { warmLedger } from './navigation'

// Each run gets its own directory (Playwright clears the output dir per run),
// and the ledger keeps no in-process state, so a second ledger over the same
// directory is what another worker process of the same run sees.
const runDir = () => join(mkdtempSync(join(tmpdir(), 'warm-ledger-')), '.warmed-routes')

describe('warmLedger', () => {
  it('reports every route cold in a fresh run', () => {
    const ledger = warmLedger(runDir())
    expect(ledger.isWarm('/profile')).toBe(false)
  })

  it('lets another worker in the same run skip a route already warmed', () => {
    const dir = runDir()
    warmLedger(dir).markWarm('/shows/e2e-attendance-test')

    const secondWorker = warmLedger(dir)
    expect(secondWorker.isWarm('/shows/e2e-attendance-test')).toBe(true)
    expect(secondWorker.isWarm('/artists/calexico')).toBe(false)
  })

  it('warms again in a new run', () => {
    warmLedger(runDir()).markWarm('/profile')
    expect(warmLedger(runDir()).isWarm('/profile')).toBe(false)
  })

  it('keeps routes that differ only in query or slashes apart', () => {
    const ledger = warmLedger(runDir())
    ledger.markWarm('/shows?page=2')
    expect(ledger.isWarm('/shows')).toBe(false)
    expect(ledger.isWarm('/shows?page=2')).toBe(true)
    ledger.markWarm('/collections/a-b')
    expect(ledger.isWarm('/collections/a/b')).toBe(false)
  })
})
