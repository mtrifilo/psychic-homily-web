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
 * pass has status 'flaky'; one that failed every attempt has 'unexpected'.
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

const OUTCOME = {
  flaky: 'passed on retry',
  unexpected: 'failed every attempt',
}

// A table cell cannot hold a raw `|` or a line break.
const cell = text => String(text).replace(/\|/g, '\\|').replace(/\r?\n/g, ' ')

/** Markdown for the job summary: the table, or one line when it is empty. */
export function retrySummaryMarkdown(retried) {
  if (retried.length === 0) {
    return '### E2E retries\n\nNo test needed a retry in this run.\n'
  }
  const rows = retried.map(test => {
    const attempts = test.attempts
      .map(attempt => `${attempt.status} ${seconds(attempt.durationMs)}`)
      .join(', ')
    const outcome = OUTCOME[test.status] ?? test.status
    return `| ${cell(test.location)} | ${cell(test.title)} | ${cell(outcome)} | ${cell(attempts)} |`
  })
  return [
    '### E2E retries',
    '',
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
