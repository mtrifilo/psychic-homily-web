// The pure half of the e2e attempt survey (attempt-survey.ts does the I/O):
// reading per-attempt results out of the two report shapes CI keeps, counting
// them per test across runs, and the table. Nothing here touches the network
// or the disk, so the counting is testable from fixtures.
//
// A test's attempts are its results in one run, one per Playwright retry. An
// attempt that never ran (status 'skipped') is left out of every count, so a
// test's first attempt is its lowest-retry attempt that ran and its final
// attempt is the highest. Any status other than 'passed' or 'skipped' is a
// failure; the suite marks no test with test.fail(), which would invert that.

import { parseArgs, stripVTControlCharacters } from 'node:util'

export interface Attempt {
  retry: number
  status: string
  /** First line of the attempt's first error, colour codes removed. */
  error?: string
}

/** One test's attempts in one run. */
export interface TestAttempts {
  file: string
  /** Describe titles, then the test title. */
  titlePath: string[]
  project: string
  attempts: Attempt[]
}

// The fields read from Playwright's JSON reporter output (the shape
// `playwright merge-reports --reporter json` writes from blob reports). The
// reporter's own JSONReport type satisfies these.
interface ReporterError {
  message?: string
}
interface ReporterResult {
  retry: number
  status?: string
  errors?: ReporterError[]
}
interface ReporterTest {
  projectName: string
  results: ReporterResult[]
}
interface ReporterSpec {
  title: string
  file: string
  tests: ReporterTest[]
}
interface ReporterSuite {
  title: string
  specs?: ReporterSpec[]
  suites?: ReporterSuite[]
}
export interface ReporterReport {
  suites?: ReporterSuite[]
}

// The fields read from the HTML reporter's data: index.html embeds a zip
// holding report.json (the file list) and one <fileId>.json per spec file
// with every test's results. The HTML report's types are not public API, so
// readHtmlReportFile checks the shape it relies on.
interface HtmlResult {
  retry: number
  status: string
  errors?: (ReporterError | string)[]
}
interface HtmlTest {
  title: string
  path: string[]
  projectName: string
  location: { file: string }
  results: HtmlResult[]
}
export interface HtmlReportFile {
  fileName: string
  tests: HtmlTest[]
}

function firstErrorLine(errors: (ReporterError | string)[] | undefined): string | undefined {
  const first = errors?.[0]
  const message = typeof first === 'string' ? first : first?.message
  if (!message) return undefined
  return stripVTControlCharacters(message)
    .split('\n')
    .map(text => text.trim())
    .find(text => text.length > 0)
}

/** One test's results in either report shape, as attempts in retry order. */
function toAttempts(results: { retry: number; status?: string; errors?: (ReporterError | string)[] }[]): Attempt[] {
  return results
    .map(result => ({
      retry: result.retry,
      status: result.status ?? 'unknown',
      error: firstErrorLine(result.errors),
    }))
    .sort((a, b) => a.retry - b.retry)
}

/** Every test in a JSON reporter report, with its attempts in retry order. */
export function attemptsFromReporterJson(report: ReporterReport): TestAttempts[] {
  const tests: TestAttempts[] = []
  const visit = (suite: ReporterSuite, describePath: string[]) => {
    for (const spec of suite.specs ?? []) {
      for (const test of spec.tests) {
        tests.push({
          file: spec.file,
          titlePath: [...describePath, spec.title],
          project: test.projectName,
          attempts: toAttempts(test.results),
        })
      }
    }
    for (const child of suite.suites ?? []) visit(child, [...describePath, child.title])
  }
  // A top-level suite is a spec file; its title is the path, which `file`
  // already carries.
  for (const fileSuite of report.suites ?? []) visit(fileSuite, [])
  return tests
}

/** The per-file data names listed by an HTML report's parsed report.json. */
export function htmlReportFileNames(index: unknown): string[] {
  const files = (index as { files?: unknown } | null)?.files
  if (!Array.isArray(files)) throw new Error('the HTML report data has no file list')
  return files.map(file => {
    const fileId = (file as { fileId?: unknown } | null)?.fileId
    if (typeof fileId !== 'string' || !/^[\w-]+$/.test(fileId)) {
      throw new Error(`the HTML report data lists a bad file id: ${String(fileId)}`)
    }
    return `${fileId}.json`
  })
}

/** Checks one parsed <fileId>.json from an HTML report and returns it typed. */
export function readHtmlReportFile(data: unknown, name: string): HtmlReportFile {
  const file = data as Partial<HtmlReportFile> | null
  if (!file || typeof file.fileName !== 'string' || !Array.isArray(file.tests)) {
    throw new Error(`${name} is not an HTML report test file (no fileName or tests)`)
  }
  for (const test of file.tests) {
    if (!Array.isArray(test?.results) || !Array.isArray(test?.path) || !test?.location?.file) {
      throw new Error(`${name} has a test without results, path or location`)
    }
  }
  return file as HtmlReportFile
}

/** Every test in an HTML report's test files, with attempts in retry order. */
export function attemptsFromHtmlReport(files: HtmlReportFile[]): TestAttempts[] {
  return files.flatMap(file =>
    file.tests.map(test => ({
      file: test.location.file,
      titlePath: [...test.path, test.title],
      project: test.projectName,
      attempts: toAttempts(test.results),
    }))
  )
}

/**
 * The zip an HTML report's index.html embeds as a base64 data URL, or null
 * when the page carries none.
 */
export function htmlReportZip(indexHtml: string): Buffer | null {
  const match = /data:application\/zip;base64,([A-Za-z0-9+/=]+)/.exec(indexHtml)
  return match ? Buffer.from(match[1], 'base64') : null
}

export type ReportSource = 'blob' | 'html' | 'partial-blob' | 'none'

export interface ArtifactInfo {
  id: number
  name: string
  expired: boolean
  sizeInBytes: number
}

export interface SourceChoice {
  source: ReportSource
  artifacts: ArtifactInfo[]
}

const MERGED_REPORT_ARTIFACT = 'playwright-report'
const BLOB_ARTIFACT = /^blob-report-(\d+)$/

/**
 * Which of a run's artifacts to read. Every shard's blob report when all
 * `shardCount` are still there; otherwise the merged HTML report; otherwise
 * whatever blobs remain, as a partial run. A re-run job uploads its artifact
 * again under the same name, and the run keeps both, so the oldest artifact
 * of a name wins: a re-run that passes must not hide the failure it re-ran.
 */
export function chooseReportSource(artifacts: ArtifactInfo[], shardCount: number): SourceChoice {
  const oldestByName = new Map<string, ArtifactInfo>()
  for (const artifact of artifacts) {
    if (artifact.expired) continue
    const seen = oldestByName.get(artifact.name)
    if (!seen || artifact.id < seen.id) oldestByName.set(artifact.name, artifact)
  }
  const blobs = [...oldestByName.values()]
    .filter(artifact => BLOB_ARTIFACT.test(artifact.name))
    .sort((a, b) => a.name.localeCompare(b.name, 'en', { numeric: true }))
  const shards = new Set(blobs.map(blob => Number(BLOB_ARTIFACT.exec(blob.name)?.[1])))
  let everyShard = true
  for (let shard = 1; shard <= shardCount; shard++) everyShard &&= shards.has(shard)
  if (everyShard) return { source: 'blob', artifacts: blobs }
  const merged = oldestByName.get(MERGED_REPORT_ARTIFACT)
  if (merged) return { source: 'html', artifacts: [merged] }
  if (blobs.length > 0) return { source: 'partial-blob', artifacts: blobs }
  return { source: 'none', artifacts: [] }
}

const isFailure = (attempt: Attempt) => attempt.status !== 'passed' && attempt.status !== 'skipped'

export interface TestTally {
  file: string
  titlePath: string[]
  project: string
  /** Runs in which at least one attempt ran. */
  runsSeen: number
  /** Runs whose first attempt that ran failed. */
  firstAttemptFailures: number
  /** Runs whose first attempt failed and final attempt passed. */
  retryRescued: number
  /** Runs whose final attempt failed. */
  finalFailures: number
  attempts: number
  failedAttempts: number
  /** First-attempt error lines, with how many runs gave each. */
  firstAttemptErrors: Map<string, number>
}

const testKey = (test: Pick<TestAttempts, 'file' | 'titlePath' | 'project'>) =>
  JSON.stringify([test.file, test.titlePath, test.project])

/** Counts each test across runs; `runs` holds one entry per run. */
export function tallyAttempts(runs: TestAttempts[][]): TestTally[] {
  const tallies = new Map<string, TestTally>()
  for (const run of runs) {
    for (const test of run) {
      const ran = test.attempts.filter(attempt => attempt.status !== 'skipped')
      if (ran.length === 0) continue
      const key = testKey(test)
      let tally = tallies.get(key)
      if (!tally) {
        tally = {
          file: test.file,
          titlePath: test.titlePath,
          project: test.project,
          runsSeen: 0,
          firstAttemptFailures: 0,
          retryRescued: 0,
          finalFailures: 0,
          attempts: 0,
          failedAttempts: 0,
          firstAttemptErrors: new Map(),
        }
        tallies.set(key, tally)
      }
      const first = ran[0]
      const final = ran[ran.length - 1]
      tally.runsSeen += 1
      tally.attempts += ran.length
      tally.failedAttempts += ran.filter(isFailure).length
      if (isFailure(final)) tally.finalFailures += 1
      if (isFailure(first)) {
        tally.firstAttemptFailures += 1
        if (!isFailure(final)) tally.retryRescued += 1
        const error = first.error ?? `(no error message; status ${first.status})`
        tally.firstAttemptErrors.set(error, (tally.firstAttemptErrors.get(error) ?? 0) + 1)
      }
    }
  }
  return [...tallies.values()]
}

const firstAttemptRate = (tally: TestTally) => tally.firstAttemptFailures / tally.runsSeen

/** Worst first-attempt failure rate first; ties by count, then by name. */
export function sortByFirstAttemptRate(tallies: TestTally[]): TestTally[] {
  return [...tallies].sort(
    (a, b) =>
      firstAttemptRate(b) - firstAttemptRate(a) ||
      b.firstAttemptFailures - a.firstAttemptFailures ||
      b.failedAttempts - a.failedAttempts ||
      testName(a).localeCompare(testName(b))
  )
}

const testName = (tally: Pick<TestTally, 'file' | 'titlePath'>) =>
  [tally.file, ...tally.titlePath].join(' > ')

const percent = (rate: number) => `${(rate * 100).toFixed(1)}%`

// A table cell holds no raw `|` or line break.
const cell = (text: string) => text.replace(/\|/g, '\\|').replace(/\r\n?|\n/g, ' ')

const MAX_ERRORS_PER_TEST = 3

/**
 * The Markdown table of `tallies` (already sorted), then each failing test's
 * most common first-attempt errors. Tests that never failed an attempt are
 * left out unless `includeClean` is set; a line says how many there were.
 */
export function surveyMarkdown(tallies: TestTally[], { includeClean = false } = {}): string {
  const projects = new Set(tallies.map(tally => tally.project))
  const name = (tally: TestTally) =>
    projects.size > 1 ? `${testName(tally)} [${tally.project}]` : testName(tally)
  const shown = includeClean ? tallies : tallies.filter(tally => tally.failedAttempts > 0)
  const hidden = tallies.length - shown.length
  const lines = [
    '| First-attempt failure rate | Runs seen | First-attempt failures | Retry-rescued runs | Final failures | Failed attempts | Test |',
    '| --- | --- | --- | --- | --- | --- | --- |',
    ...shown.map(
      tally =>
        `| ${percent(firstAttemptRate(tally))} | ${tally.runsSeen} | ${tally.firstAttemptFailures} | ${tally.retryRescued} | ${tally.finalFailures} | ${tally.failedAttempts} of ${tally.attempts} | ${cell(name(tally))} |`
    ),
  ]
  if (hidden > 0) {
    lines.push('', `${hidden} other ${hidden === 1 ? 'test' : 'tests'} ran with no failed attempt.`)
  }
  const withErrors = shown.filter(tally => tally.firstAttemptErrors.size > 0)
  if (withErrors.length > 0) {
    lines.push('', 'First-attempt errors (first line, runs):')
    for (const tally of withErrors) {
      lines.push('', `- ${name(tally)}`)
      const common = [...tally.firstAttemptErrors.entries()]
        .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
        .slice(0, MAX_ERRORS_PER_TEST)
      for (const [error, count] of common) lines.push(`  - ${count}x ${error}`)
      const rest = tally.firstAttemptErrors.size - common.length
      if (rest > 0) lines.push(`  - ${rest} other distinct errors`)
    }
  }
  return `${lines.join('\n')}\n`
}

export const USAGE = `Usage: bun e2e/tools/attempt-survey.ts [options]

Counts every e2e test's per-attempt results over the latest completed CI runs
of a branch, from the runs' Playwright report artifacts, and prints per test:
runs seen, first-attempt failures, retry-rescued runs, final failures and
failed attempts, worst first-attempt failure rate first. Needs an
authenticated gh, unzip on PATH, and frontend's dependencies installed (the
blob reports are merged with its Playwright).

Options:
  --runs N            how many completed runs to read (default 20)
  --branch NAME       branch whose runs to read (default main)
  --through-run ID    newest run to read; the N runs end at this one
  --shards N          shard count of the e2e job (default 4); a run with a
                      blob report for every shard is read from the blobs,
                      otherwise from the merged HTML report
  --work-dir DIR      keep each run's parsed results here and reuse them on
                      the next invocation (default: a temporary directory,
                      removed at exit)
  --all               also list tests that never failed an attempt
  --dry-run           list the runs and the artifacts that would be read;
                      download nothing
  -h, --help          print this help
`

export interface SurveyOptions {
  runs: number
  branch: string
  throughRun?: number
  shards: number
  workDir?: string
  all: boolean
  dryRun: boolean
  help: boolean
}

function positiveInteger(value: string, flag: string): number {
  if (!/^\d+$/.test(value) || Number(value) < 1) {
    throw new Error(`${flag} takes a positive integer, not '${value}'`)
  }
  return Number(value)
}

/** Parses the command line; throws on an unknown flag or a bad value. */
export function parseSurveyArgs(argv: string[]): SurveyOptions {
  const { values } = parseArgs({
    args: argv,
    options: {
      runs: { type: 'string', default: '20' },
      branch: { type: 'string', default: 'main' },
      'through-run': { type: 'string' },
      shards: { type: 'string', default: '4' },
      'work-dir': { type: 'string' },
      all: { type: 'boolean', default: false },
      'dry-run': { type: 'boolean', default: false },
      help: { type: 'boolean', short: 'h', default: false },
    },
  })
  return {
    runs: positiveInteger(values.runs, '--runs'),
    branch: values.branch,
    throughRun:
      values['through-run'] === undefined
        ? undefined
        : positiveInteger(values['through-run'], '--through-run'),
    shards: positiveInteger(values.shards, '--shards'),
    workDir: values['work-dir'],
    all: values.all,
    dryRun: values['dry-run'],
    help: values.help,
  }
}
