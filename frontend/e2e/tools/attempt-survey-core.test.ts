import { describe, expect, it } from 'vitest'
import {
  type ArtifactInfo,
  type HtmlReportFile,
  type ReporterReport,
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

const FILE = 'pages/example.spec.ts'
// The same three errors in both report shapes; MOVED carries colour codes.
const MOVED = 'Error: the dot \u001b[31mmoved\u001b[39m\n\nExpected: < 458'
const TIMEOUT = '\nTest timeout of 30000ms exceeded.'
const BROKEN = 'Error: still broken'

type Result = { retry: number; status: string; message?: string }

const results = (...attempts: Result[]) =>
  attempts.map(({ retry, status, message }) => ({
    retry,
    status,
    errors: message ? [{ message }] : [],
  }))

// One run's JSON reporter report (what merge-reports writes from blobs): a
// top-level test, a describe block, and a nested describe. Results are listed
// out of retry order on purpose.
const reporterReport: ReporterReport = {
  suites: [
    {
      title: FILE,
      specs: [
        {
          title: 'passes first time',
          file: FILE,
          tests: [{ projectName: 'chromium', results: results({ retry: 0, status: 'passed' }) }],
        },
      ],
      suites: [
        {
          title: 'Example',
          specs: [
            {
              title: 'rescued by a retry',
              file: FILE,
              tests: [
                {
                  projectName: 'chromium',
                  results: results(
                    { retry: 1, status: 'passed' },
                    { retry: 0, status: 'failed', message: MOVED }
                  ),
                },
              ],
            },
          ],
          suites: [
            {
              title: 'nested',
              specs: [
                {
                  title: 'fails every attempt',
                  file: FILE,
                  tests: [
                    {
                      projectName: 'chromium',
                      results: results(
                        { retry: 0, status: 'timedOut', message: TIMEOUT },
                        { retry: 1, status: 'failed', message: BROKEN },
                        { retry: 2, status: 'failed', message: BROKEN }
                      ),
                    },
                  ],
                },
                {
                  title: 'is skipped',
                  file: FILE,
                  tests: [{ projectName: 'chromium', results: results({ retry: 0, status: 'skipped' }) }],
                },
              ],
            },
          ],
        },
      ],
    },
  ],
}

// The same run as the HTML report's per-file data holds it.
const htmlTest = (title: string, path: string[], attempts: Result[]) => ({
  title,
  path,
  projectName: 'chromium',
  location: { file: FILE },
  results: results(...attempts),
})

const htmlFiles: HtmlReportFile[] = [
  {
    fileName: FILE,
    tests: [
      htmlTest('passes first time', [], [{ retry: 0, status: 'passed' }]),
      htmlTest(
        'rescued by a retry',
        ['Example'],
        [
          { retry: 0, status: 'failed', message: MOVED },
          { retry: 1, status: 'passed' },
        ]
      ),
      htmlTest(
        'fails every attempt',
        ['Example', 'nested'],
        [
          { retry: 0, status: 'timedOut', message: TIMEOUT },
          { retry: 1, status: 'failed', message: BROKEN },
          { retry: 2, status: 'failed', message: BROKEN },
        ]
      ),
      htmlTest('is skipped', ['Example', 'nested'], [{ retry: 0, status: 'skipped' }]),
    ],
  },
]

describe('attemptsFromReporterJson', () => {
  it('names each test by its describe path and orders its attempts by retry', () => {
    const tests = attemptsFromReporterJson(reporterReport)
    expect(tests.map(test => test.titlePath)).toEqual([
      ['passes first time'],
      ['Example', 'rescued by a retry'],
      ['Example', 'nested', 'fails every attempt'],
      ['Example', 'nested', 'is skipped'],
    ])
    expect(tests[1]).toEqual({
      file: FILE,
      titlePath: ['Example', 'rescued by a retry'],
      project: 'chromium',
      attempts: [
        { retry: 0, status: 'failed', error: 'Error: the dot moved' },
        { retry: 1, status: 'passed', error: undefined },
      ],
    })
    expect(tests[2].attempts[0].error).toBe('Test timeout of 30000ms exceeded.')
  })

  it('reads nothing from a report without suites', () => {
    expect(attemptsFromReporterJson({})).toEqual([])
  })
})

describe('attemptsFromHtmlReport', () => {
  it('reads the same tests and attempts as the JSON reporter for the same run', () => {
    expect(attemptsFromHtmlReport(htmlFiles)).toEqual(attemptsFromReporterJson(reporterReport))
  })

  it('takes an error given as a plain string', () => {
    const [test] = attemptsFromHtmlReport([
      {
        fileName: FILE,
        tests: [
          {
            ...htmlTest('t', [], []),
            results: [{ retry: 0, status: 'failed', errors: ['Error: plain\nmore'] }],
          },
        ],
      },
    ])
    expect(test.attempts[0].error).toBe('Error: plain')
  })
})

describe('readHtmlReportFile', () => {
  it('accepts a test file and rejects anything without the fields it reads', () => {
    expect(readHtmlReportFile(htmlFiles[0], 'a.json')).toBe(htmlFiles[0])
    expect(() => readHtmlReportFile({ files: [] }, 'report.json')).toThrow(/report.json is not/)
    expect(() =>
      readHtmlReportFile({ fileName: FILE, tests: [{ title: 'no results' }] }, 'b.json')
    ).toThrow(/b.json has a test without results/)
  })
})

describe('htmlReportFileNames', () => {
  it('names each listed file data file and rejects a bad list', () => {
    expect(htmlReportFileNames({ files: [{ fileId: 'a1-b2' }, { fileId: 'c3' }] })).toEqual([
      'a1-b2.json',
      'c3.json',
    ])
    expect(() => htmlReportFileNames({})).toThrow('no file list')
    expect(() => htmlReportFileNames({ files: [{ fileId: '../x' }] })).toThrow('bad file id: ../x')
  })
})

describe('htmlReportZip', () => {
  it('decodes the zip embedded in index.html', () => {
    const payload = Buffer.from('PK zip bytes')
    const html = `<script id="playwrightReportBase64" type="application/zip">data:application/zip;base64,${payload.toString('base64')}</script>`
    expect(htmlReportZip(html)).toEqual(payload)
  })

  it('returns null for a page without one', () => {
    expect(htmlReportZip('<html></html>')).toBeNull()
  })
})

describe('tallyAttempts', () => {
  // Run 1 is the fixture above. Run 2: the rescued test passes first time and
  // the failing test is rescued on its last retry.
  const run2 = attemptsFromReporterJson(reporterReport).map(test => {
    if (test.titlePath.at(-1) === 'rescued by a retry') {
      return { ...test, attempts: [{ retry: 0, status: 'passed' }] }
    }
    if (test.titlePath.at(-1) === 'fails every attempt') {
      return {
        ...test,
        attempts: [
          { retry: 0, status: 'failed', error: 'Error: still broken' },
          { retry: 1, status: 'interrupted' },
          { retry: 2, status: 'passed' },
        ],
      }
    }
    return test
  })
  const tallies = tallyAttempts([attemptsFromReporterJson(reporterReport), run2])
  const tally = (title: string) => tallies.find(entry => entry.titlePath.at(-1) === title)

  it('counts a pass, a retry-rescued run and a final failure', () => {
    expect(tally('passes first time')).toMatchObject({
      runsSeen: 2,
      firstAttemptFailures: 0,
      retryRescued: 0,
      finalFailures: 0,
      attempts: 2,
      failedAttempts: 0,
    })
    expect(tally('rescued by a retry')).toMatchObject({
      runsSeen: 2,
      firstAttemptFailures: 1,
      retryRescued: 1,
      finalFailures: 0,
      attempts: 3,
      failedAttempts: 1,
    })
    expect(tally('fails every attempt')).toMatchObject({
      runsSeen: 2,
      firstAttemptFailures: 2,
      retryRescued: 1,
      finalFailures: 1,
      attempts: 6,
      failedAttempts: 5,
    })
  })

  it('counts first-attempt errors by their first line', () => {
    expect(tally('rescued by a retry')?.firstAttemptErrors).toEqual(
      new Map([['Error: the dot moved', 1]])
    )
    expect(tally('fails every attempt')?.firstAttemptErrors).toEqual(
      new Map([
        ['Test timeout of 30000ms exceeded.', 1],
        ['Error: still broken', 1],
      ])
    )
  })

  it('leaves out a test none of whose attempts ran', () => {
    expect(tally('is skipped')).toBeUndefined()
  })

  it('starts from the first attempt that ran when retry 0 was skipped', () => {
    const [only] = tallyAttempts([
      [
        {
          file: FILE,
          titlePath: ['serial sibling'],
          project: 'chromium',
          attempts: [
            { retry: 0, status: 'skipped' },
            { retry: 1, status: 'passed' },
          ],
        },
      ],
    ])
    expect(only).toMatchObject({ runsSeen: 1, firstAttemptFailures: 0, attempts: 1 })
  })

  it('sorts the worst first-attempt failure rate first', () => {
    expect(sortByFirstAttemptRate(tallies).map(entry => entry.titlePath.at(-1))).toEqual([
      'fails every attempt',
      'rescued by a retry',
      'passes first time',
    ])
  })
})

describe('surveyMarkdown', () => {
  const tallies = sortByFirstAttemptRate(tallyAttempts([attemptsFromReporterJson(reporterReport)]))

  it('lists tests with a failed attempt, the rest as a count, then their errors', () => {
    expect(surveyMarkdown(tallies)).toBe(
      [
        '| First-attempt failure rate | Runs seen | First-attempt failures | Retry-rescued runs | Final failures | Failed attempts | Test |',
        '| --- | --- | --- | --- | --- | --- | --- |',
        `| 100.0% | 1 | 1 | 0 | 1 | 3 of 3 | ${FILE} > Example > nested > fails every attempt |`,
        `| 100.0% | 1 | 1 | 1 | 0 | 1 of 2 | ${FILE} > Example > rescued by a retry |`,
        '',
        '1 other test ran with no failed attempt.',
        '',
        'First-attempt errors (first line, runs):',
        '',
        `- ${FILE} > Example > nested > fails every attempt`,
        '  - 1x Test timeout of 30000ms exceeded.',
        '',
        `- ${FILE} > Example > rescued by a retry`,
        '  - 1x Error: the dot moved',
        '',
      ].join('\n')
    )
  })

  it('lists every test with includeClean, and escapes a pipe in a title', () => {
    const piped = tallies.map(entry => ({ ...entry, titlePath: [...entry.titlePath.slice(0, -1), 'a | b'] }))
    const markdown = surveyMarkdown(piped, { includeClean: true })
    expect(markdown).toContain(`| 0.0% | 1 | 0 | 0 | 0 | 0 of 1 | ${FILE} > a \\| b |`)
    expect(markdown).not.toContain('other tests ran')
  })
})

describe('chooseReportSource', () => {
  let nextId = 1
  const artifact = (name: string, overrides: Partial<ArtifactInfo> = {}): ArtifactInfo => ({
    id: nextId++,
    name,
    expired: false,
    sizeInBytes: 1000,
    ...overrides,
  })
  const blobs = (...shards: number[]) => shards.map(shard => artifact(`blob-report-${shard}`))
  const names = (artifacts: ArtifactInfo[]) => artifacts.map(entry => entry.name)

  it('reads every shard blob when all are present, in shard order', () => {
    const choice = chooseReportSource([artifact('playwright-report'), ...blobs(10, 2, 1, 3, 4)], 4)
    expect(choice.source).toBe('blob')
    expect(names(choice.artifacts)).toEqual([
      'blob-report-1',
      'blob-report-2',
      'blob-report-3',
      'blob-report-4',
      'blob-report-10',
    ])
  })

  it('falls back to the merged report when a shard blob is gone', () => {
    const choice = chooseReportSource([...blobs(1, 2, 4), artifact('playwright-report')], 4)
    expect(choice).toEqual({ source: 'html', artifacts: [expect.objectContaining({ name: 'playwright-report' })] })
  })

  it('reads the blobs it has when there is no merged report, as partial', () => {
    expect(chooseReportSource(blobs(1, 3), 4).source).toBe('partial-blob')
    expect(chooseReportSource([artifact('playwright-report', { expired: true })], 4).source).toBe('none')
    expect(chooseReportSource([], 4)).toEqual({ source: 'none', artifacts: [] })
  })

  it('takes the first execution of a re-run job, not the re-run', () => {
    const first = artifact('blob-report-4')
    const rerun = artifact('blob-report-4')
    const choice = chooseReportSource([...blobs(1, 2, 3), rerun, first], 4)
    expect(choice.artifacts.find(entry => entry.name === 'blob-report-4')?.id).toBe(first.id)
  })
})

describe('parseSurveyArgs', () => {
  it('defaults to the last 20 main runs over 4 shards', () => {
    expect(parseSurveyArgs([])).toEqual({
      runs: 20,
      branch: 'main',
      throughRun: undefined,
      shards: 4,
      workDir: undefined,
      all: false,
      dryRun: false,
      help: false,
    })
  })

  it('reads every option', () => {
    expect(
      parseSurveyArgs([
        '--runs',
        '29',
        '--branch',
        'release',
        '--through-run',
        '38651969015',
        '--shards',
        '6',
        '--work-dir',
        '/tmp/survey',
        '--all',
        '--dry-run',
        '-h',
      ])
    ).toEqual({
      runs: 29,
      branch: 'release',
      throughRun: 38651969015,
      shards: 6,
      workDir: '/tmp/survey',
      all: true,
      dryRun: true,
      help: true,
    })
  })

  it('rejects a bad count and an unknown flag', () => {
    expect(() => parseSurveyArgs(['--runs', '0'])).toThrow('--runs takes a positive integer')
    expect(() => parseSurveyArgs(['--runs', '2x'])).toThrow('--runs takes a positive integer')
    expect(() => parseSurveyArgs(['--since', 'monday'])).toThrow()
  })
})
