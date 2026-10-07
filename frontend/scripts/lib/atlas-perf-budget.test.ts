import { spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { AFTER_MAP_WINDOW_MS, DEFAULT_BUDGET, EXIT, TARGET, budgetTable, errorMessage, judge, median, resolveBudget, runBudgetCheck, targetLine } from './atlas-perf-budget.mjs'

const MIB = 1024 * 1024

type Run = {
  firstMapMs: number
  entry: { totalBytes: number }
  afterMap: { totalBytes: number }
  city: { readyMs: number; totalBytes: number }
  rasterRequests: number
}

const run = (firstMapMs: number, entryBytes = 1.4 * MIB, rasterRequests = 0, afterMapBytes = 64 * 1024): Run => ({
  firstMapMs,
  entry: { totalBytes: entryBytes },
  afterMap: { totalBytes: afterMapBytes },
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

const check = (results: Array<Run | Error>, overrides: Record<string, unknown> = {}) => {
  const runOnce = stubRunner(results)
  const report = vi.fn()
  const onError = vi.fn()
  const code = runBudgetCheck({
    count: results.length,
    runOnce,
    budget: DEFAULT_BUDGET,
    compact: true,
    enforce: true,
    report,
    onError,
    ...overrides,
  })
  return { code, runOnce, report, onError }
}

describe('atlas perf budget', () => {
  it('defaults to the owner budget: first map 3.5 s, entry 1.5 MiB; the target is 2.5 s', () => {
    expect(DEFAULT_BUDGET).toEqual({ firstMapMs: 3500, entryBytes: 1.5 * MIB })
    expect(TARGET).toEqual({ firstMapMs: 2500 })
  })

  it('keeps the exit codes the workflow summary and the script header name', () => {
    expect(EXIT).toEqual({ PASS: 0, BUDGET_MISSED: 1, HARNESS_ERROR: 2 })
  })

  it('takes the median of odd and even run counts', () => {
    expect(median([3600, 3300, 3400])).toBe(3400)
    expect(median([3300, 3500])).toBe(3400)
  })

  describe('resolveBudget', () => {
    it('defaults to DEFAULT_BUDGET', () => {
      expect(resolveBudget({})).toEqual({ budget: DEFAULT_BUDGET })
      expect(resolveBudget()).toEqual({ budget: DEFAULT_BUDGET })
    })

    it('applies --budget-ms and --budget-bytes independently', () => {
      expect(resolveBudget({ firstMapMs: '2500' })).toEqual({ budget: { firstMapMs: 2500, entryBytes: DEFAULT_BUDGET.entryBytes } })
      expect(resolveBudget({ entryBytes: '1048576' })).toEqual({ budget: { firstMapMs: DEFAULT_BUDGET.firstMapMs, entryBytes: MIB } })
    })

    it.each(['0', '-1', '3.5', 'abc', '', '1e3', '0x10', ' 5', '9007199254740993'])('rejects %j', (raw) => {
      expect(resolveBudget({ firstMapMs: raw })).toEqual({ error: '--budget-ms must be a positive integer' })
      expect(resolveBudget({ entryBytes: raw })).toEqual({ error: '--budget-bytes must be a positive integer' })
    })
  })

  describe('judge', () => {
    it('passes at exactly the limits (at or under)', () => {
      const verdict = judge([run(3500, 1.5 * MIB)], { budget: DEFAULT_BUDGET, compact: true })
      expect(verdict).toMatchObject({ firstMapOk: true, entryOk: true, rasterOk: true, pass: true })
    })

    it('fails 1 ms or 1 byte over a limit', () => {
      expect(judge([run(3501)], { budget: DEFAULT_BUDGET, compact: true })).toMatchObject({ firstMapOk: false, pass: false })
      expect(judge([run(3000, 1.5 * MIB + 1)], { budget: DEFAULT_BUDGET, compact: true })).toMatchObject({ entryOk: false, pass: false })
    })

    it('passes on the median when one run is slow', () => {
      const verdict = judge([run(4500), run(3300), run(3400)], { budget: DEFAULT_BUDGET, compact: true })
      expect(verdict).toMatchObject({ firstMapMs: 3400, pass: true })
    })

    it('fails when the median itself is over', () => {
      const verdict = judge([run(3953), run(3383), run(3502)], { budget: DEFAULT_BUDGET, compact: true })
      expect(verdict).toMatchObject({ firstMapMs: 3502, pass: false })
    })

    it('fails a compact viewport on a raster request in any run, and ignores raster on desktop', () => {
      const runs = [run(3000), run(3000, 1.4 * MIB, 2), run(3000)]
      expect(judge(runs, { budget: DEFAULT_BUDGET, compact: true })).toMatchObject({ rasterRequests: 2, rasterOk: false, pass: false })
      expect(judge(runs, { budget: DEFAULT_BUDGET, compact: false })).toMatchObject({ rasterOk: true, pass: true })
    })

    it('reports the 2.5 s target delta without gating on it', () => {
      const over = judge([run(3380)], { budget: DEFAULT_BUDGET, compact: true })
      expect(over.target).toEqual({ firstMapMs: 2500, deltaMs: 880 })
      expect(over.pass).toBe(true)
      expect(judge([run(2400)], { budget: DEFAULT_BUDGET, compact: true }).target.deltaMs).toBe(-100)
    })

    it.each([
      [880, 'the median is 0.88 s over it.'],
      [-100, 'the median is 0.10 s under it.'],
      [0, 'the median is exactly at it.'],
    ])('prints the target line for a %d ms delta', (deltaMs, ending) => {
      expect(targetLine({ firstMapMs: 2500, deltaMs })).toBe(`Target (reported, not gated): first rendered map at most 2.50 s; ${ending}`)
    })

    it('leaves first map out of pass when it is informational, still judging it', () => {
      const note = 'SwiftShader, informational'
      const slow = judge([run(6075)], { budget: DEFAULT_BUDGET, compact: true, firstMapNote: note })
      expect(slow).toMatchObject({ firstMapOk: false, firstMapNote: note, pass: true })
      const heavy = judge([run(6075, 1.5 * MIB + 1)], { budget: DEFAULT_BUDGET, compact: true, firstMapNote: note })
      expect(heavy.pass).toBe(false)
      const raster = judge([run(6075, 1.4 * MIB, 1)], { budget: DEFAULT_BUDGET, compact: true, firstMapNote: note })
      expect(raster.pass).toBe(false)
    })

    it('prints the owner limit on every row, with the note in place of the first-map result', () => {
      const gated = budgetTable(judge([run(3400, 1.4 * MIB)], { budget: DEFAULT_BUDGET, compact: true }))
      expect(gated).toEqual([
        '| Budget | Value (median unless noted) | Limit | Result |',
        '|---|---:|---:|---|',
        '| First rendered map | 3.40 s | 3.50 s | PASS |',
        '| Entry bytes (to first map) | 1.40 MiB | 1.50 MiB | PASS |',
        '| Bytes in the first 3 s after first map | 0.06 MiB | none | reported only |',
        '| Raster requests (compact viewport, worst run) | 0 | 0 | PASS |',
      ])
      const note = 'SwiftShader, informational, about 1.75x local GPU'
      const ci = budgetTable(judge([run(6075, 1.5 * MIB + 1)], { budget: DEFAULT_BUDGET, compact: false, firstMapNote: note }))
      expect(ci).toEqual([
        '| Budget | Value (median unless noted) | Limit | Result |',
        '|---|---:|---:|---|',
        `| First rendered map | 6.08 s | 3.50 s | ${note} |`,
        '| Entry bytes (to first map) | 1.50 MiB | 1.50 MiB | FAIL |',
        '| Bytes in the first 3 s after first map | 0.06 MiB | none | reported only |',
      ])
    })

    it('reports the after-map bytes as a median and never gates on them', () => {
      const heavyAfter = judge(
        [run(3000, 1.4 * MIB, 0, 2 * MIB), run(3100, 1.4 * MIB, 0, 3 * MIB), run(3200, 1.4 * MIB, 0, 4 * MIB)],
        { budget: DEFAULT_BUDGET, compact: true },
      )
      expect(heavyAfter).toMatchObject({ afterMapBytes: 3 * MIB, entryOk: true, pass: true })
      expect(AFTER_MAP_WINDOW_MS).toBe(3000)
    })

    it('checks against an overridden budget', () => {
      const { budget } = resolveBudget({ firstMapMs: '2500' })
      expect(judge([run(3000)], { budget, compact: true })).toMatchObject({ budget: { firstMapMs: 2500 }, pass: false })
    })
  })

  describe('runBudgetCheck exit codes', () => {
    it('exits 0 when the medians meet the budget, after one call per run and one report', async () => {
      const { code, runOnce, report, onError } = check([run(3300), run(3400), run(3500)])
      expect(await code).toBe(EXIT.PASS)
      expect(runOnce).toHaveBeenCalledTimes(3)
      expect(report).toHaveBeenCalledTimes(1)
      expect(report.mock.calls[0][1]).toMatchObject({ firstMapMs: 3400, pass: true })
      expect(onError).not.toHaveBeenCalled()
    })

    it('exits 1 when the budget is missed', async () => {
      const { code, report } = check([run(3600), run(3700), run(3400)])
      expect(await code).toBe(EXIT.BUDGET_MISSED)
      expect(report).toHaveBeenCalledTimes(1)
    })

    it('exits 0 on a missed budget when not enforced (--no-budget)', async () => {
      const { code, report } = check([run(3600), run(3700), run(3400)], { enforce: false })
      expect(await code).toBe(EXIT.PASS)
      expect(report.mock.calls[0][1].pass).toBe(false)
    })

    it('exits 0 on a slow first map that is informational, and 1 when entry bytes miss', async () => {
      const note = 'SwiftShader, informational'
      expect(await check([run(6057), run(6415), run(6075)], { firstMapNote: note }).code).toBe(EXIT.PASS)
      expect(await check([run(6075, 1.6 * MIB)], { firstMapNote: note }).code).toBe(EXIT.BUDGET_MISSED)
    })

    it('exits 1 under an overridden budget the default would pass', async () => {
      const { budget } = resolveBudget({ firstMapMs: '2500' })
      const { code } = check([run(3000)], { budget })
      expect(await code).toBe(EXIT.BUDGET_MISSED)
    })

    it('exits 2 on the first harness error, without further runs or a report', async () => {
      const failure = new Error('the map never became ready')
      const { code, runOnce, report, onError } = check([run(3300), failure, run(3300)])
      expect(await code).toBe(EXIT.HARNESS_ERROR)
      expect(runOnce).toHaveBeenCalledTimes(2)
      expect(onError).toHaveBeenCalledWith(failure)
      expect(report).not.toHaveBeenCalled()
    })

    it('exits 2, not 1, when the report itself throws (an unwritable --json path)', async () => {
      const failure = new Error('ENOENT')
      const report = vi.fn(() => {
        throw failure
      })
      const { code, onError } = check([run(3300)], { report })
      expect(await code).toBe(EXIT.HARNESS_ERROR)
      expect(onError).toHaveBeenCalledWith(failure)
    })

    it('reads a message from a thrown non-Error too', () => {
      expect(errorMessage(new Error('boom'))).toBe('boom')
      expect(errorMessage('boom')).toBe('boom')
      expect(errorMessage(undefined)).toBe('undefined')
    })

    it('exits 2 on a harness error even when the budget is not enforced', async () => {
      const { code } = check([new Error('scene list')], { enforce: false })
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
      [['https://example.test', '--first-map-informational', ' '], '--first-map-informational needs a non-empty note'],
    ])('exits 2 for %j', (args, message) => {
      const result = cli(...args)
      expect(result.status).toBe(EXIT.HARNESS_ERROR)
      expect(result.stderr).toContain(message)
    })
  })
})
