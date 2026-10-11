import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { collectRetriedTests, retrySummaryMarkdown, retryWarningLine } from './e2e-retry-table.mjs'

type Attempt = { status: string; duration: number }

const spec = (title: string, line: number, status: string, attempts: Attempt[]) => ({
  title,
  file: 'pages/example.spec.ts',
  line,
  tests: [{ status, results: attempts }],
})

// The JSON reporter's tree: a file suite (title = path, no specs of its own
// here) holding a describe block that holds the specs, plus a nested describe.
const report = {
  suites: [
    {
      title: 'pages/example.spec.ts',
      file: 'pages/example.spec.ts',
      specs: [],
      suites: [
        {
          title: 'Example',
          specs: [
            spec('passes first time', 5, 'expected', [{ status: 'passed', duration: 1200 }]),
            spec('flakes | once', 12, 'flaky', [
              { status: 'failed', duration: 10_600 },
              { status: 'passed', duration: 12_414 },
            ]),
          ],
          suites: [
            {
              title: 'nested',
              specs: [
                spec('fails every attempt', 30, 'unexpected', [
                  { status: 'timedOut', duration: 30_012 },
                  { status: 'failed', duration: 21_991 },
                  { status: 'failed', duration: 7_803 },
                ]),
                spec('is skipped', 40, 'skipped', [{ status: 'skipped', duration: 0 }]),
              ],
            },
          ],
        },
      ],
    },
  ],
}

describe('collectRetriedTests', () => {
  it('lists only tests with more than one attempt, with every attempt', () => {
    expect(collectRetriedTests(report)).toEqual([
      {
        location: 'pages/example.spec.ts:12',
        title: 'Example > flakes | once',
        status: 'flaky',
        attempts: [
          { status: 'failed', durationMs: 10_600 },
          { status: 'passed', durationMs: 12_414 },
        ],
      },
      {
        location: 'pages/example.spec.ts:30',
        title: 'Example > nested > fails every attempt',
        status: 'unexpected',
        attempts: [
          { status: 'timedOut', durationMs: 30_012 },
          { status: 'failed', durationMs: 21_991 },
          { status: 'failed', durationMs: 7_803 },
        ],
      },
    ])
  })

  it('returns nothing for a run without retries or without suites', () => {
    const clean = { suites: [{ title: 'pages/example.spec.ts', specs: [spec('passes', 1, 'expected', [{ status: 'passed', duration: 1 }])] }] }
    expect(collectRetriedTests(clean)).toEqual([])
    expect(collectRetriedTests({})).toEqual([])
  })
})

describe('retrySummaryMarkdown', () => {
  it('writes one row per retried test, escaping a pipe in the title', () => {
    const markdown = retrySummaryMarkdown(collectRetriedTests(report))
    expect(markdown).toContain('2 tests needed more than one attempt.')
    expect(markdown).toContain('| pages/example.spec.ts:12 | Example &gt; flakes \\| once | passed on retry | failed 10.6 s, passed 12.4 s |')
    expect(markdown).toContain('| pages/example.spec.ts:30 | Example &gt; nested &gt; fails every attempt | failed every attempt | timedOut 30.0 s, failed 22.0 s, failed 7.8 s |')
  })

  it('says so in one line when nothing was retried', () => {
    expect(retrySummaryMarkdown([])).toBe('### E2E retries\n\nNo test needed a retry in this run.\n')
  })

  it('labels serial-group collateral and attempts that never ran', () => {
    // One test in a serial group fails once: the whole group is retried, so a
    // sibling that passed is run twice and a later one is skipped, then run.
    const serial = {
      suites: [
        {
          title: 'pages/example.spec.ts',
          suites: [
            {
              title: 'Serial',
              specs: [
                spec('passed before the failure', 3, 'expected', [
                  { status: 'passed', duration: 2_100 },
                  { status: 'passed', duration: 1_900 },
                ]),
                spec('ran after the failure', 9, 'expected', [
                  { status: 'skipped', duration: 0 },
                  { status: 'passed', duration: 3_000 },
                ]),
              ],
            },
          ],
        },
      ],
    }
    const markdown = retrySummaryMarkdown(collectRetriedTests(serial))
    expect(markdown).toContain('| Serial &gt; passed before the failure | passed; retried with its serial group | passed 2.1 s, passed 1.9 s |')
    expect(markdown).toContain('| Serial &gt; ran after the failure | passed; retried with its serial group | not run, passed 3.0 s |')
  })

  it('keeps HTML and carriage returns in a title from reaching the markdown raw', () => {
    const title = { location: 'a.spec.ts:1', title: 'invalid /shows/<slug> & more\rnext', status: 'flaky', attempts: [] }
    expect(retrySummaryMarkdown([title])).toContain('| invalid /shows/&lt;slug&gt; &amp; more next |')
  })

  it('says the report is partial when shard blobs are missing', () => {
    const partial = 'Partial report: report-3.zip missing, so tests in those shards are not listed.'
    expect(retrySummaryMarkdown([], { missingShards: ['report-3.zip'] })).toContain(partial)
    expect(retrySummaryMarkdown(collectRetriedTests(report), { missingShards: ['report-3.zip'] })).toContain(partial)
    expect(retrySummaryMarkdown([])).not.toContain('Partial report')
  })
})

describe('retryWarningLine', () => {
  it('is one ::warning line with the count, and null for an empty table', () => {
    const retried = collectRetriedTests(report)
    expect(retryWarningLine(retried)).toBe(
      '::warning title=E2E retries::2 tests needed more than one attempt; see the E2E retries table in the job summary'
    )
    expect(retryWarningLine(retried.slice(0, 1))).toMatch(/::1 test needed/)
    expect(retryWarningLine([])).toBeNull()
  })
})

describe('e2e-retry-summary.mjs', () => {
  const script = join(dirname(fileURLToPath(import.meta.url)), '..', 'e2e-retry-summary.mjs')
  const run = (args: string[], summaryFile?: string, extraEnv: Record<string, string> = {}) =>
    spawnSync(process.execPath, [script, ...args], {
      encoding: 'utf8',
      env: { ...process.env, GITHUB_STEP_SUMMARY: summaryFile ?? '', MISSING_SHARD_BLOBS: '', ...extraEnv },
    })

  it('appends the table to the step summary only, warns once and exits 0', () => {
    const dir = mkdtempSync(join(tmpdir(), 'e2e-retry-'))
    const reportFile = join(dir, 'report.json')
    const summaryFile = join(dir, 'summary.md')
    writeFileSync(reportFile, JSON.stringify(report))
    writeFileSync(summaryFile, '')

    const result = run([reportFile], summaryFile)

    expect(result.status).toBe(0)
    expect(readFileSync(summaryFile, 'utf8')).toContain('| Spec | Test | Outcome | Attempts |')
    // Titles stay out of step output, which the runner parses for commands.
    expect(result.stdout).not.toContain('flakes')
    expect(result.stdout.match(/^::warning /gm)).toHaveLength(1)
  })

  it('names missing shard blobs from MISSING_SHARD_BLOBS', () => {
    const dir = mkdtempSync(join(tmpdir(), 'e2e-retry-'))
    const reportFile = join(dir, 'report.json')
    const summaryFile = join(dir, 'summary.md')
    writeFileSync(reportFile, JSON.stringify({ suites: [] }))
    writeFileSync(summaryFile, '')

    const result = run([reportFile], summaryFile, { MISSING_SHARD_BLOBS: 'report-2.zip report-4.zip' })

    expect(result.status).toBe(0)
    expect(readFileSync(summaryFile, 'utf8')).toContain('Partial report: report-2.zip, report-4.zip missing')
  })

  it('exits 0 without a warning when no test was retried', () => {
    const dir = mkdtempSync(join(tmpdir(), 'e2e-retry-'))
    const reportFile = join(dir, 'report.json')
    writeFileSync(reportFile, JSON.stringify({ suites: [] }))

    const result = run([reportFile])

    expect(result.status).toBe(0)
    expect(result.stdout).toContain('No test needed a retry in this run.')
    expect(result.stdout).not.toContain('::warning')
  })

  it('warns and exits 1 when the report is missing or malformed', () => {
    const dir = mkdtempSync(join(tmpdir(), 'e2e-retry-'))
    const malformed = join(dir, 'malformed.json')
    writeFileSync(malformed, '{"suites": 3}')

    for (const args of [[], [join(dir, 'absent.json')], [malformed]]) {
      const result = run(args)
      expect(result.status).toBe(1)
      expect(result.stdout).toMatch(/^::warning title=E2E retries::retry table unavailable: /m)
    }
  })
})
