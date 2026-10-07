import { spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { EXIT, GATE, TARGET, judge, median, resolveBudget, runGate } from './atlas-perf-gate.mjs'

const MIB = 1024 * 1024

type Run = {
  firstMapMs: number
  entry: { totalBytes: number }
  city: { readyMs: number; totalBytes: number }
  rasterRequests: number
}

const run = (firstMapMs: number, entryBytes = 1.4 * MIB, rasterRequests = 0): Run => ({
  firstMapMs,
  entry: { totalBytes: entryBytes },
  city: { readyMs: 600, totalBytes: 279 * 1024 },
  rasterRequests,
})

/** A runner that returns the given runs in order, or throws a given error. */
const stubRunner = (results: Array<Run | Error>) => {
  let i = 0
  return vi.fn(async () => {
    const next = results[i++]
    if (next instanceof Error) throw next
    return next
  })
}

const gate = (results: Array<Run | Error>, overrides: Record<string, unknown> = {}) => {
  const runOnce = stubRunner(results)
  const report = vi.fn()
  const onError = vi.fn()
  const code = runGate({
    count: results.length,
    runOnce,
    budget: GATE,
    compact: true,
    enforce: true,
    report,
    onError,
    ...overrides,
  })
  return { code, runOnce, report, onError }
}

describe('atlas perf gate', () => {
  it('is the owner gate: first map 3.5 s, entry 1.5 MiB; the target is 2.5 s', () => {
    expect(GATE).toEqual({ firstMapMs: 3500, entryBytes: 1.5 * MIB })
    expect(TARGET).toEqual({ firstMapMs: 2500 })
  })

  it('takes the median of odd and even run counts', () => {
    expect(median([3600, 3300, 3400])).toBe(3400)
    expect(median([3300, 3500])).toBe(3400)
  })

  describe('resolveBudget', () => {
    it('defaults to the gate', () => {
      expect(resolveBudget({})).toEqual({ budget: GATE })
      expect(resolveBudget()).toEqual({ budget: GATE })
    })

    it('applies --budget-ms and --budget-bytes independently', () => {
      expect(resolveBudget({ firstMapMs: '2500' })).toEqual({ budget: { firstMapMs: 2500, entryBytes: GATE.entryBytes } })
      expect(resolveBudget({ entryBytes: '1048576' })).toEqual({ budget: { firstMapMs: GATE.firstMapMs, entryBytes: MIB } })
    })

    it.each(['0', '-1', '3.5', 'abc', ''])('rejects %j', (raw) => {
      expect(resolveBudget({ firstMapMs: raw })).toEqual({ error: '--budget-ms must be a positive integer' })
      expect(resolveBudget({ entryBytes: raw })).toEqual({ error: '--budget-bytes must be a positive integer' })
    })
  })

  describe('judge', () => {
    it('passes at exactly the limits (at or under)', () => {
      const verdict = judge([run(3500, 1.5 * MIB)], { budget: GATE, compact: true })
      expect(verdict).toMatchObject({ firstMapOk: true, entryOk: true, rasterOk: true, pass: true })
    })

    it('fails 1 ms or 1 byte over a limit', () => {
      expect(judge([run(3501)], { budget: GATE, compact: true })).toMatchObject({ firstMapOk: false, pass: false })
      expect(judge([run(3000, 1.5 * MIB + 1)], { budget: GATE, compact: true })).toMatchObject({ entryOk: false, pass: false })
    })

    it('checks the medians, so one slow run does not fail the gate', () => {
      const verdict = judge([run(3953), run(3383), run(3502)], { budget: GATE, compact: true })
      expect(verdict.firstMapMs).toBe(3502)
      expect(verdict.pass).toBe(false)
      expect(judge([run(4500), run(3300), run(3400)], { budget: GATE, compact: true }).pass).toBe(true)
    })

    it('fails a compact viewport on a raster request in any run, and ignores raster on desktop', () => {
      const runs = [run(3000), run(3000, 1.4 * MIB, 2), run(3000)]
      expect(judge(runs, { budget: GATE, compact: true })).toMatchObject({ rasterRequests: 2, rasterOk: false, pass: false })
      expect(judge(runs, { budget: GATE, compact: false })).toMatchObject({ rasterOk: true, pass: true })
    })

    it('reports the 2.5 s target delta without gating on it', () => {
      const over = judge([run(3380)], { budget: GATE, compact: true })
      expect(over.target).toEqual({ firstMapMs: 2500, deltaMs: 880 })
      expect(over.pass).toBe(true)
      expect(judge([run(2400)], { budget: GATE, compact: true }).target.deltaMs).toBe(-100)
    })

    it('checks against an overridden budget', () => {
      const { budget } = resolveBudget({ firstMapMs: '2500' })
      expect(judge([run(3000)], { budget, compact: true })).toMatchObject({ budget: { firstMapMs: 2500 }, pass: false })
    })
  })

  describe('runGate exit codes', () => {
    it('exits 0 when the medians meet the budget, after one call per run and one report', async () => {
      const { code, runOnce, report, onError } = gate([run(3300), run(3400), run(3500)])
      expect(await code).toBe(EXIT.PASS)
      expect(runOnce).toHaveBeenCalledTimes(3)
      expect(report).toHaveBeenCalledTimes(1)
      expect(report.mock.calls[0][1]).toMatchObject({ firstMapMs: 3400, pass: true })
      expect(onError).not.toHaveBeenCalled()
    })

    it('exits 1 when the budget is missed', async () => {
      const { code, report } = gate([run(3600), run(3700), run(3400)])
      expect(await code).toBe(EXIT.BUDGET_MISSED)
      expect(report).toHaveBeenCalledTimes(1)
    })

    it('exits 0 on a missed budget when not enforced (--no-budget)', async () => {
      const { code, report } = gate([run(3600), run(3700), run(3400)], { enforce: false })
      expect(await code).toBe(EXIT.PASS)
      expect(report.mock.calls[0][1].pass).toBe(false)
    })

    it('exits 1 under an overridden budget the default would pass', async () => {
      const { budget } = resolveBudget({ firstMapMs: '2500' })
      const { code } = gate([run(3000)], { budget })
      expect(await code).toBe(EXIT.BUDGET_MISSED)
    })

    it('exits 2 on the first harness error, without further runs or a report', async () => {
      const failure = new Error('the map never passed the readiness gate')
      const { code, runOnce, report, onError } = gate([run(3300), failure, run(3300)])
      expect(await code).toBe(EXIT.HARNESS_ERROR)
      expect(runOnce).toHaveBeenCalledTimes(2)
      expect(onError).toHaveBeenCalledWith(failure)
      expect(report).not.toHaveBeenCalled()
    })

    it('exits 2 on a harness error even when the budget is not enforced', async () => {
      const { code } = gate([new Error('scene list')], { enforce: false })
      expect(await code).toBe(EXIT.HARNESS_ERROR)
    })
  })

  // Argument errors exit before any browser launches, so the real CLI runs here.
  describe('atlas-perf.mjs argument errors', () => {
    const script = join(dirname(fileURLToPath(import.meta.url)), '..', 'atlas-perf.mjs')
    const cli = (...args: string[]) => spawnSync(process.execPath, [script, ...args], { encoding: 'utf8', env: { ...process.env, VERCEL_PROTECTION_BYPASS: '' } })

    it.each([
      [['https://example.test', '--budget-ms', '0'], '--budget-ms must be a positive integer'],
      [['https://example.test', '--budget-bytes', '1.5'], '--budget-bytes must be a positive integer'],
      [['https://example.test', '--budget-ms'], '--budget-ms needs a value'],
    ])('exits 2 for %j', (args, message) => {
      const result = cli(...args)
      expect(result.status).toBe(EXIT.HARNESS_ERROR)
      expect(result.stderr).toContain(message)
    })
  })
})
