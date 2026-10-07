// The Atlas perf budget, verdict and exit code. The browser harness in
// scripts/atlas-perf.mjs supplies the runs; everything here is pure, so the
// decision is testable without a browser.

const MIB = 1024 * 1024

// The limits the script exits on (medians of the runs).
export const DEFAULT_BUDGET = {
  firstMapMs: 3500,
  entryBytes: 1.5 * MIB,
}

// Reported with its delta beside the budget; never part of the exit code.
export const TARGET = {
  firstMapMs: 2500,
}

// The workflow's job summary and the script header name these numbers.
export const EXIT = {
  PASS: 0,
  BUDGET_MISSED: 1,
  HARNESS_ERROR: 2,
}

export const median = (values) => {
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

/**
 * DEFAULT_BUDGET with any flag overrides applied. Each override is the raw
 * flag value (a string of decimal digits) or undefined. Returns { budget } or
 * { error }.
 */
export function resolveBudget({ firstMapMs, entryBytes } = {}) {
  const budget = { ...DEFAULT_BUDGET }
  for (const [key, flag, raw] of [
    ['firstMapMs', '--budget-ms', firstMapMs],
    ['entryBytes', '--budget-bytes', entryBytes],
  ]) {
    if (raw === undefined) continue
    const value = /^\d+$/.test(raw) ? Number(raw) : NaN
    if (!Number.isSafeInteger(value) || value < 1) return { error: `${flag} must be a positive integer` }
    budget[key] = value
  }
  return { budget }
}

/**
 * Medians of the runs checked against `budget`. A compact viewport also
 * fails on any raster request in any run (`rasterRequests` is the worst run).
 */
export function judge(runs, { budget, compact }) {
  const firstMapMs = median(runs.map((r) => r.firstMapMs))
  const entryBytes = median(runs.map((r) => r.entry.totalBytes))
  const rasterRequests = Math.max(...runs.map((r) => r.rasterRequests))
  const firstMapOk = firstMapMs <= budget.firstMapMs
  const entryOk = entryBytes <= budget.entryBytes
  const rasterOk = !compact || rasterRequests === 0
  return {
    budget,
    target: { firstMapMs: TARGET.firstMapMs, deltaMs: firstMapMs - TARGET.firstMapMs },
    firstMapMs,
    entryBytes,
    cityMs: median(runs.map((r) => r.city.readyMs)),
    cityBytes: median(runs.map((r) => r.city.totalBytes)),
    compact,
    rasterRequests,
    firstMapOk,
    entryOk,
    rasterOk,
    pass: firstMapOk && entryOk && rasterOk,
  }
}

export const seconds = (ms) => (ms / 1000).toFixed(2)

/** The report's target line for a verdict's `target`. */
export function targetLine({ firstMapMs, deltaMs }) {
  const where = deltaMs === 0 ? 'exactly at it' : `${seconds(Math.abs(deltaMs))} s ${deltaMs > 0 ? 'over' : 'under'} it`
  return `Target (reported, not gated): first rendered map at most ${seconds(firstMapMs)} s; the median is ${where}.`
}

/**
 * Runs `runOnce` `count` times in sequence and returns the exit code. A run
 * that throws, or a `report` that throws, ends the check with HARNESS_ERROR
 * after `onError`; otherwise `report` receives the runs and the verdict, and
 * the code is BUDGET_MISSED only when `enforce` is set and the verdict fails.
 */
export async function runBudgetCheck({ count, runOnce, budget, compact, enforce, report, onError }) {
  const runs = []
  try {
    for (let i = 0; i < count; i++) runs.push(await runOnce())
    const verdict = judge(runs, { budget, compact })
    report(runs, verdict)
    return enforce && !verdict.pass ? EXIT.BUDGET_MISSED : EXIT.PASS
  } catch (error) {
    onError(error)
    return EXIT.HARNESS_ERROR
  }
}

/** A thrown value's message, for values that are not Error instances too. */
export const errorMessage = (thrown) => (thrown instanceof Error ? thrown.message : String(thrown))
