#!/usr/bin/env node
// Writes the E2E retry table (scripts/lib/e2e-retry-table.mjs) for a
// Playwright JSON report to the job summary, and prints one ::warning line
// when any test took more than one attempt.
//
//   node scripts/e2e-retry-summary.mjs <report.json>
//
// The summary goes to the file named by GITHUB_STEP_SUMMARY when it is set,
// and to stdout either way. Exit codes: 0 once the report is read, retries or
// not (the table warns, it never gates); 1 when the report cannot be read or
// has no suites, after a ::warning saying so.

import { appendFileSync, readFileSync } from 'node:fs'
import {
  collectRetriedTests,
  retrySummaryMarkdown,
  retryWarningLine,
} from './lib/e2e-retry-table.mjs'

function unavailable(reason) {
  // Workflow-command data must not carry a raw newline, `%` or carriage return.
  const data = String(reason)
    .replace(/%/g, '%25')
    .replace(/\r/g, '%0D')
    .replace(/\n/g, '%0A')
  console.log(`::warning title=E2E retries::retry table unavailable: ${data}`)
  process.exit(1)
}

const reportPath = process.argv[2]
if (!reportPath) unavailable('no report path given')

let report
try {
  report = JSON.parse(readFileSync(reportPath, 'utf8'))
} catch (err) {
  unavailable(`cannot read ${reportPath}: ${err.message}`)
}
if (!Array.isArray(report?.suites)) {
  unavailable(`${reportPath} has no suites array`)
}

const retried = collectRetriedTests(report)
const markdown = retrySummaryMarkdown(retried)
console.log(markdown)
if (process.env.GITHUB_STEP_SUMMARY) {
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${markdown}\n`)
}
const warning = retryWarningLine(retried)
if (warning) console.log(warning)
