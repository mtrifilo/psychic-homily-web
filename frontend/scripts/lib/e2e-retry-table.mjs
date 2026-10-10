// The E2E retry table: every test in a Playwright JSON report that took more
// than one attempt. scripts/e2e-retry-summary.mjs reads the report and writes
// the result to the CI job summary; everything here is pure, so the table is
// testable without a report on disk.
//
// Input is the built-in JSON reporter's shape: a tree of suites (file suites,
// then describe blocks), each with `specs`, each spec with one `tests` entry
// per project, each test with one `results` entry per attempt.

/**
 * Every test with more than one attempt, in report order. A test retried to a
 * pass has status 'flaky', one that failed every attempt has 'unexpected', and
 * one retried only alongside a failing serial-group sibling has 'expected'.
 */
export function collectRetriedTests(report) {
  const retried = []
  const visit = (suite, titlePath) => {
    for (const spec of suite.specs ?? []) {
      for (const test of spec.tests ?? []) {
        const results = test.results ?? []
        if (results.length < 2) continue
        retried.push({
          location: `${spec.file}:${spec.line}`,
          title: [...titlePath, spec.title].join(' > '),
          status: test.status,
          attempts: results.map(result => ({
            status: result.status,
            durationMs: result.duration,
          })),
        })
      }
    }
    for (const child of suite.suites ?? []) {
      visit(child, [...titlePath, child.title])
    }
  }
  // A file suite's title is its path, which `location` already carries.
  for (const fileSuite of report.suites ?? []) visit(fileSuite, [])
  return retried
}

const seconds = ms => `${(ms / 1000).toFixed(1)} s`

const testCount = n => `${n} ${n === 1 ? 'test' : 'tests'}`

// 'expected' with more than one attempt is a test that passed every attempt
// it ran but was retried because another test in its serial group failed.
const OUTCOME = {
  flaky: 'passed on retry',
  unexpected: 'failed every attempt',
  expected: 'passed; retried with its serial group',
}

// A skipped attempt in a multi-attempt test is one that never ran: the rest
// of its serial group was skipped after a failure.
const attemptText = attempt =>
  attempt.status === 'skipped'
    ? 'not run'
    : `${attempt.status} ${seconds(attempt.durationMs)}`

// A table cell holds no raw `|`, line break or HTML.
const cell = text =>
  String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\|/g, '\\|')
    .replace(/\r\n?|\n/g, ' ')

const partialLine = missingShards =>
  `Partial report: ${missingShards.join(', ')} missing, so tests in those shards are not listed.`

/**
 * Markdown for the job summary: the table, or one line when it is empty.
 * `missingShards` names shard blobs the merge did not have.
 */
export function retrySummaryMarkdown(retried, { missingShards = [] } = {}) {
  const partial = missingShards.length > 0 ? [partialLine(missingShards), ''] : []
  if (retried.length === 0) {
    return ['### E2E retries', '', ...partial, 'No test needed a retry in this run.', ''].join('\n')
  }
  const rows = retried.map(test => {
    const attempts = test.attempts.map(attemptText).join(', ')
    const outcome = OUTCOME[test.status] ?? test.status
    return `| ${cell(test.location)} | ${cell(test.title)} | ${cell(outcome)} | ${cell(attempts)} |`
  })
  return [
    '### E2E retries',
    '',
    ...partial,
    `${testCount(retried.length)} needed more than one attempt. A test that passed on retry is a flake the job conclusion does not show.`,
    '',
    '| Spec | Test | Outcome | Attempts |',
    '| --- | --- | --- | --- |',
    ...rows,
    '',
  ].join('\n')
}

/**
 * The single workflow-command line for a non-empty table, or null. The text
 * is fixed apart from the count, so it needs no workflow-command escaping.
 */
export function retryWarningLine(retried) {
  if (retried.length === 0) return null
  return `::warning title=E2E retries::${testCount(retried.length)} needed more than one attempt; see the E2E retries table in the job summary`
}
