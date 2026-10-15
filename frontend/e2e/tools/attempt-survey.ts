// Survey e2e flake rates per attempt, from the Playwright report artifacts of
// a branch's latest CI runs. A job that goes green on a retry hides a failed
// first attempt; this counts attempts instead. Usage: see USAGE, or --help.
//
// GitHub is read with `gh` only: the run list, and each run's artifact list
// and artifact zips through `gh api`. Locally it also needs unzip and the
// frontend's own Playwright, which merges blob reports. The table goes to
// stdout, progress and per-run problems to stderr.

import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  type ArtifactInfo,
  type SourceChoice,
  type SurveyOptions,
  type TestAttempts,
  USAGE,
  attemptsFromHtmlReport,
  attemptsFromReporterJson,
  chooseReportSource,
  htmlReportFileNames,
  htmlReportZip,
  parseSurveyArgs,
  readHtmlReportFile,
  sortByFirstAttemptRate,
  surveyMarkdown,
  tallyAttempts,
} from './attempt-survey-core'

const WORKFLOW = 'ci.yml'
// Runs created in the same second as --through-run's can list ahead of it.
const SAME_SECOND_SLACK = 10

const FRONTEND_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const PLAYWRIGHT_BIN = join(FRONTEND_DIR, 'node_modules', '.bin', 'playwright')
const PARSED_RESULTS = 'tests.json'

interface Run {
  databaseId: number
  attempt: number
  headSha: string
  createdAt: string
  conclusion: string
}

interface PlannedRun {
  target: Run
  choice: SourceChoice
}

function runForBytes(command: string, args: string[], env?: Record<string, string>): Buffer {
  try {
    return execFileSync(command, args, {
      cwd: FRONTEND_DIR,
      env: { ...process.env, ...env },
      maxBuffer: 256 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
  } catch (err) {
    const failure = err as { stderr?: Buffer; message: string }
    const detail = failure.stderr?.toString('utf8').trim() || failure.message
    throw new Error(`${command} ${args.join(' ')} failed: ${detail}`)
  }
}

const run = (command: string, args: string[], env?: Record<string, string>) =>
  runForBytes(command, args, env).toString('utf8')

function listRuns(options: SurveyOptions): Run[] {
  const filters = ['--workflow', WORKFLOW, '--branch', options.branch, '--status', 'completed']
  const fields = ['--json', 'databaseId,attempt,headSha,createdAt,conclusion']
  if (options.throughRun === undefined) {
    return JSON.parse(run('gh', ['run', 'list', ...filters, '--limit', String(options.runs), ...fields]))
  }
  const createdAt = run('gh', ['run', 'view', String(options.throughRun), '--json', 'createdAt', '--jq', '.createdAt']).trim()
  const runs = JSON.parse(
    run('gh', [
      'run',
      'list',
      ...filters,
      '--created',
      `<=${createdAt}`,
      '--limit',
      String(options.runs + SAME_SECOND_SLACK),
      ...fields,
    ])
  ) as Run[]
  const start = runs.findIndex(candidate => candidate.databaseId === options.throughRun)
  if (start === -1) {
    throw new Error(`run ${options.throughRun} is not a completed ${WORKFLOW} run on ${options.branch}`)
  }
  return runs.slice(start, start + options.runs)
}

const listArtifacts = (runId: number): ArtifactInfo[] =>
  JSON.parse(
    run('gh', [
      'api',
      `repos/{owner}/{repo}/actions/runs/${runId}/artifacts?per_page=100`,
      '--jq',
      '[.artifacts[] | {id, name, expired, sizeInBytes: .size_in_bytes}]',
    ])
  )

// By id, not by name: a re-run job leaves two artifacts with one name.
function download(artifact: ArtifactInfo, dir: string, members: string[] = []) {
  const zipPath = join(dirname(dir), `artifact-${artifact.id}.zip`)
  writeFileSync(zipPath, runForBytes('gh', ['api', `repos/{owner}/{repo}/actions/artifacts/${artifact.id}/zip`]))
  run('unzip', ['-q', '-o', zipPath, ...members, '-d', dir])
}

function readBlobRun(artifacts: ArtifactInfo[], runDir: string): TestAttempts[] {
  const blobDir = join(runDir, 'blobs')
  for (const artifact of artifacts) download(artifact, blobDir)
  const merged = join(runDir, 'merged.json')
  run(PLAYWRIGHT_BIN, ['merge-reports', '--reporter', 'json', blobDir], {
    PLAYWRIGHT_JSON_OUTPUT_FILE: merged,
  })
  return attemptsFromReporterJson(JSON.parse(readFileSync(merged, 'utf8')))
}

function readHtmlRun([report]: ArtifactInfo[], runDir: string): TestAttempts[] {
  const htmlDir = join(runDir, 'html')
  download(report, htmlDir, ['index.html'])
  const zip = htmlReportZip(readFileSync(join(htmlDir, 'index.html'), 'utf8'))
  if (!zip) throw new Error('the HTML report embeds no report data')
  const zipPath = join(runDir, 'html-data.zip')
  const dataDir = join(runDir, 'html-data')
  writeFileSync(zipPath, zip)
  run('unzip', ['-q', '-o', zipPath, '-d', dataDir])
  const readJson = (name: string): unknown => JSON.parse(readFileSync(join(dataDir, name), 'utf8'))
  const files = htmlReportFileNames(readJson('report.json')).map(name =>
    readHtmlReportFile(readJson(name), name)
  )
  return attemptsFromHtmlReport(files)
}

/**
 * The run's tests. The work dir keeps only each run's parsed results, which
 * an invocation with the same --work-dir reuses: a run's chosen artifacts are
 * its first execution's, which a later re-run does not change.
 */
function readRun({ target, choice }: PlannedRun, workDir: string): TestAttempts[] {
  const runDir = join(workDir, String(target.databaseId))
  const parsed = join(runDir, PARSED_RESULTS)
  if (existsSync(parsed)) return JSON.parse(readFileSync(parsed, 'utf8'))
  rmSync(runDir, { recursive: true, force: true })
  mkdirSync(runDir, { recursive: true })
  const tests =
    choice.source === 'html' ? readHtmlRun(choice.artifacts, runDir) : readBlobRun(choice.artifacts, runDir)
  writeFileSync(parsed, JSON.stringify(tests))
  for (const entry of readdirSync(runDir)) {
    if (entry !== PARSED_RESULTS) rmSync(join(runDir, entry), { recursive: true, force: true })
  }
  return tests
}

// A re-run's conclusion is its latest attempt's.
const conclusion = (target: Run) =>
  target.attempt > 1 ? `${target.conclusion} on attempt ${target.attempt}` : target.conclusion

const shortSha = (target: Run) => target.headSha.slice(0, 9)

const describeRun = (target: Run) =>
  `${target.databaseId} (${shortSha(target)}, ${target.createdAt}, ${conclusion(target)})`

const megabytes = (bytes: number) => `${(bytes / 1024 / 1024).toFixed(1)} MB`

function printDryRun(plan: PlannedRun[]) {
  const lines = ['| Run | Head | Created | Conclusion | Source | Artifacts |', '| --- | --- | --- | --- | --- | --- |']
  for (const { target, choice } of plan) {
    const listed = choice.artifacts.map(artifact => `${artifact.name} ${megabytes(artifact.sizeInBytes)}`).join(', ')
    lines.push(
      `| ${target.databaseId} | ${shortSha(target)} | ${target.createdAt} | ${conclusion(target)} | ${choice.source} | ${listed || 'none'} |`
    )
  }
  console.log(lines.join('\n'))
}

function survey(options: SurveyOptions) {
  if (!existsSync(PLAYWRIGHT_BIN) && !options.dryRun) {
    throw new Error(`${PLAYWRIGHT_BIN} is missing; run bun install in frontend/`)
  }
  const runs = listRuns(options)
  if (runs.length === 0) throw new Error(`no completed ${WORKFLOW} runs on ${options.branch}`)
  const plan = runs.map(target => ({
    target,
    choice: chooseReportSource(listArtifacts(target.databaseId), options.shards),
  }))
  if (options.dryRun) {
    printDryRun(plan)
    return
  }

  const workDir = options.workDir ?? mkdtempSync(join(tmpdir(), 'e2e-attempt-survey-'))
  const read: TestAttempts[][] = []
  const bySource = new Map<string, number>()
  const unread: string[] = []
  try {
    plan.forEach((planned, index) => {
      const { target, choice } = planned
      console.error(`[${index + 1}/${plan.length}] ${describeRun(target)}: ${choice.source}`)
      if (choice.source === 'none') {
        unread.push(`${target.databaseId} (no report artifact)`)
        return
      }
      try {
        read.push(readRun(planned, workDir))
        bySource.set(choice.source, (bySource.get(choice.source) ?? 0) + 1)
      } catch (err) {
        console.error(`  unreadable: ${(err as Error).message}`)
        unread.push(`${target.databaseId} (unreadable)`)
      }
    })
  } finally {
    if (!options.workDir) rmSync(workDir, { recursive: true, force: true })
  }

  const sources = [...bySource.entries()].map(([source, count]) => `${count} ${source}`).join(', ')
  const lines = [
    `E2E attempt survey: ${runs.length} completed ${WORKFLOW} runs on ${options.branch}, ${describeRun(runs[runs.length - 1])} to ${describeRun(runs[0])}.`,
    `Read ${read.length} runs (${sources || 'none'}).`,
  ]
  if (unread.length > 0) lines.push(`Not read: ${unread.join(', ')}.`)
  const rerun = runs.filter(target => target.attempt > 1)
  if (rerun.length > 0) {
    const listed = rerun.map(target => `${target.databaseId} (attempt ${target.attempt})`).join(', ')
    lines.push(`Re-run: ${listed}; each is read from its first execution's artifacts.`)
  }
  lines.push('', surveyMarkdown(sortByFirstAttemptRate(tallyAttempts(read)), { includeClean: options.all }))
  console.log(lines.join('\n'))
}

let options: SurveyOptions
try {
  options = parseSurveyArgs(process.argv.slice(2))
} catch (err) {
  console.error(`${(err as Error).message}\n\n${USAGE}`)
  process.exit(2)
}
if (options.help) {
  console.log(USAGE)
} else {
  try {
    survey(options)
  } catch (err) {
    console.error((err as Error).message)
    process.exit(1)
  }
}
