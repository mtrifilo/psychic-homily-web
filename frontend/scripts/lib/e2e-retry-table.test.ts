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
  tests: [{ projectName: 'chromium', status, results: attempts.map((a, retry) => ({ ...a, retry })) }],
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
        projectName: 'chromium',
        status: 'flaky',
        attempts: [
          { status: 'failed', durationMs: 10_600 },
          { status: 'passed', durationMs: 12_414 },
        ],
      },
      {
        location: 'pages/example.spec.ts:30',
        title: 'Example > nested > fails every attempt',
        projectName: 'chromium',
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
    const clean = { suites: [{ ...report.suites[0], suites: [{ title: 'Example', specs: [report.suites[0].suites[0].specs[0]] }] }] }
    expect(collectRetriedTests(clean)).toEqual([])
    expect(collectRetriedTests({})).toEqual([])
  })
})

describe('retrySummaryMarkdown', () => {
  it('writes one row per retried test, escaping a pipe in the title', () => {
    const markdown = retrySummaryMarkdown(collectRetriedTests(report))
    expect(markdown).toContain('2 tests needed more than one attempt.')
    expect(markdown).toContain('| pages/example.spec.ts:12 | Example > flakes \\| once | passed on retry | failed 10.6 s, passed 12.4 s |')
    expect(markdown).toContain('| pages/example.spec.ts:30 | Example > nested > fails every attempt | failed every attempt | timedOut 30.0 s, failed 22.0 s, failed 7.8 s |')
  })

  it('says so in one line when nothing was retried', () => {
    expect(retrySummaryMarkdown([])).toBe('### E2E retries\n\nNo test needed a retry in this run.\n')
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
  const run = (args: string[], summaryFile?: string) =>
    spawnSync(process.execPath, [script, ...args], {
      encoding: 'utf8',
      env: { ...process.env, GITHUB_STEP_SUMMARY: summaryFile ?? '' },
    })

  it('appends the table to the step summary, warns once and exits 0', () => {
    const dir = mkdtempSync(join(tmpdir(), 'e2e-retry-'))
    const reportFile = join(dir, 'report.json')
    const summaryFile = join(dir, 'summary.md')
    writeFileSync(reportFile, JSON.stringify(report))
    writeFileSync(summaryFile, '')

    const result = run([reportFile], summaryFile)

    expect(result.status).toBe(0)
    expect(readFileSync(summaryFile, 'utf8')).toContain('| Spec | Test | Outcome | Attempts |')
    expect(result.stdout.match(/^::warning /gm)).toHaveLength(1)
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
