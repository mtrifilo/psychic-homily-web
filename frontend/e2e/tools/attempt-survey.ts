// Survey e2e flake rates per attempt, from the Playwright report artifacts of
// a branch's latest CI runs. A job that goes green on a retry hides a failed
// first attempt; this counts attempts instead. Usage: see USAGE, or --help.
//
// Reads GitHub with `gh` only (the run list, and each run's artifact list and
// artifact zips through `gh api`), so it needs nothing beyond an authenticated
// gh. The table goes to stdout, progress and per-run problems to stderr.

import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
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
  htmlReportZip,
  parseSurveyArgs,
  readHtmlReportFile,
  sortByFirstAttemptRate,
  surveyMarkdown,
  tallyAttempts,
} from './attempt-survey-core'

const WORKFLOW = 'ci.yml'
// How far past --through-run's position the run list may need to reach.
const THROUGH_RUN_SEARCH = 300

const FRONTEND_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const PLAYWRIGHT_BIN = join(FRONTEND_DIR, 'node_modules', '.bin', 'playwright')

interface Run {
  databaseId: number
  attempt: number
  headSha: string
  createdAt: string
  conclusion: string
}

function runForBytes(command: string, args: string[], env?: Record<string, string>): Buffer {
  try {
    return execFileSync(command, args, {
      cwd: FRONTEND_DIR,
      env: env ? { ...process.env, ...env } : process.env,
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
  const limit = options.throughRun === undefined ? options.runs : options.runs + THROUGH_RUN_SEARCH
  const runs = JSON.parse(
    run('gh', [
      'run',
      'list',
      '--workflow',
      WORKFLOW,
      '--branch',
      options.branch,
      '--status',
      'completed',
      '--limit',
      String(limit),
      '--json',
      'databaseId,attempt,headSha,createdAt,conclusion',
    ])
  ) as Run[]
  if (options.throughRun === undefined) return runs
  const start = runs.findIndex(candidate => candidate.databaseId === options.throughRun)
  if (start === -1) {
    throw new Error(
      `run ${options.throughRun} is not among the latest ${limit} completed ${WORKFLOW} runs on ${options.branch}`
    )
  }
  return runs.slice(start, start + options.runs)
}

function listArtifacts(runId: number): ArtifactInfo[] {
  const artifacts = JSON.parse(
    run('gh', [
      'api',
      `repos/{owner}/{repo}/actions/runs/${runId}/artifacts?per_page=100`,
      '--jq',
      '[.artifacts[] | {id, name, expired, sizeInBytes: .size_in_bytes}]',
    ])
  ) as ArtifactInfo[]
  return artifacts
}

// By id, not by name: a re-run job leaves two artifacts with one name.
function download(artifact: ArtifactInfo, dir: string) {
  const zipPath = join(dirname(dir), `artifact-${artifact.id}.zip`)
  writeFileSync(zipPath, runForBytes('gh', ['api', `repos/{owner}/{repo}/actions/artifacts/${artifact.id}/zip`]))
  run('unzip', ['-q', '-o', zipPath, '-d', dir])
}

function readBlobRun(choice: SourceChoice, runDir: string): TestAttempts[] {
  const blobDir = join(runDir, 'blobs')
  for (const artifact of choice.artifacts) download(artifact, blobDir)
  const merged = join(runDir, 'merged.json')
  run(PLAYWRIGHT_BIN, ['merge-reports', '--reporter', 'json', blobDir], {
    PLAYWRIGHT_JSON_OUTPUT_FILE: merged,
  })
  return attemptsFromReporterJson(JSON.parse(readFileSync(merged, 'utf8')))
}

function readHtmlRun(choice: SourceChoice, runDir: string): TestAttempts[] {
  const htmlDir = join(runDir, 'html')
  download(choice.artifacts[0], htmlDir)
  const zip = htmlReportZip(readFileSync(join(htmlDir, 'index.html'), 'utf8'))
  if (!zip) throw new Error('the HTML report embeds no report data')
  const zipPath = join(runDir, 'html-data.zip')
  const dataDir = join(runDir, 'html-data')
  writeFileSync(zipPath, zip)
  run('unzip', ['-q', '-o', zipPath, '-d', dataDir])
  const index = JSON.parse(readFileSync(join(dataDir, 'report.json'), 'utf8')) as {
    files?: { fileId?: unknown }[]
  }
  if (!Array.isArray(index.files)) throw new Error('the HTML report data has no file list')
  const files = index.files.map(({ fileId }) => {
    if (typeof fileId !== 'string' || !/^[\w-]+$/.test(fileId)) {
      throw new Error(`the HTML report data lists a bad file id: ${String(fileId)}`)
    }
    const name = `${fileId}.json`
    return readHtmlReportFile(JSON.parse(readFileSync(join(dataDir, name), 'utf8')), name)
  })
  return attemptsFromHtmlReport(files)
}

/**
 * The run's tests, from the work dir's copy when an earlier invocation with
 * the same --work-dir already read this run. A run's chosen artifacts are its
 * first execution's, which a later re-run does not change.
 */
function readRun(target: Run, choice: SourceChoice, workDir: string): TestAttempts[] {
  const runDir = join(workDir, String(target.databaseId))
  const cached = join(runDir, 'tests.json')
  if (existsSync(cached)) return JSON.parse(readFileSync(cached, 'utf8'))
  rmSync(runDir, { recursive: true, force: true })
  mkdirSync(runDir, { recursive: true })
  const tests =
    choice.source === 'html'
      ? readHtmlRun(choice, runDir)
      : readBlobRun(choice, runDir)
  writeFileSync(cached, JSON.stringify(tests))
  return tests
}

// A re-run's conclusion is its latest attempt's.
const conclusion = (target: Run) =>
  target.attempt > 1 ? `${target.conclusion} on attempt ${target.attempt}` : target.conclusion

const describeRun = (target: Run) =>
  `${target.databaseId} (${target.headSha.slice(0, 9)}, ${target.createdAt}, ${conclusion(target)})`

const megabytes = (bytes: number) => `${(bytes / 1024 / 1024).toFixed(1)} MB`

function dryRun(runs: Run[], choices: SourceChoice[]) {
  const lines = ['| Run | Head | Created | Conclusion | Source | Artifacts |', '| --- | --- | --- | --- | --- | --- |']
  runs.forEach((target, index) => {
    const { source, artifacts } = choices[index]
    const listed = artifacts.map(artifact => `${artifact.name} ${megabytes(artifact.sizeInBytes)}`).join(', ')
    lines.push(
      `| ${target.databaseId} | ${target.headSha.slice(0, 9)} | ${target.createdAt} | ${conclusion(target)} | ${source} | ${listed || 'none'} |`
    )
  })
  console.log(lines.join('\n'))
}

function survey(options: SurveyOptions) {
  if (!existsSync(PLAYWRIGHT_BIN) && !options.dryRun) {
    throw new Error(`${PLAYWRIGHT_BIN} is missing; run bun install in frontend/`)
  }
  const runs = listRuns(options)
  if (runs.length === 0) throw new Error(`no completed ${WORKFLOW} runs on ${options.branch}`)
  const choices = runs.map(target => chooseReportSource(listArtifacts(target.databaseId), options.shards))
  if (options.dryRun) {
    dryRun(runs, choices)
    return
  }

  const workDir = options.workDir ?? mkdtempSync(join(tmpdir(), 'e2e-attempt-survey-'))
  const read: TestAttempts[][] = []
  const bySource = new Map<string, number>()
  const unread: string[] = []
  try {
    runs.forEach((target, index) => {
      const choice = choices[index]
      console.error(`[${index + 1}/${runs.length}] ${describeRun(target)}: ${choice.source}`)
      if (choice.source === 'none') {
        unread.push(`${target.databaseId} (no report artifact)`)
        return
      }
      try {
        read.push(readRun(target, choice, workDir))
        bySource.set(choice.source, (bySource.get(choice.source) ?? 0) + 1)
      } catch (err) {
        console.error(`  unreadable: ${(err as Error).message}`)
        unread.push(`${target.databaseId} (unreadable)`)
      }
    })
  } finally {
    if (!options.workDir) rmSync(workDir, { recursive: true, force: true })
  }

  const newest = runs[0]
  const oldest = runs[runs.length - 1]
  const sources = [...bySource.entries()].map(([source, count]) => `${count} ${source}`).join(', ')
  const rerun = runs.filter(target => target.attempt > 1)
  console.log(
    [
      `E2E attempt survey: ${runs.length} completed ${WORKFLOW} runs on ${options.branch}, ${describeRun(oldest)} to ${describeRun(newest)}.`,
      `Read ${read.length} runs (${sources || 'none'}).${unread.length > 0 ? ` Not read: ${unread.join(', ')}.` : ''}`,
      ...(rerun.length > 0
        ? [
            `Re-run: ${rerun.map(target => `${target.databaseId} (attempt ${target.attempt})`).join(', ')}; each is read from its first execution's artifacts.`,
          ]
        : []),
      '',
      surveyMarkdown(sortByFirstAttemptRate(tallyAttempts(read)), { includeClean: options.all }),
    ].join('\n')
  )
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
