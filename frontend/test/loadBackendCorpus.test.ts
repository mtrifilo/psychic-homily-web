import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { loadBackendCorpus } from './loadBackendCorpus'

const backendRoot = resolve(import.meta.dirname, '../../backend')

describe('loadBackendCorpus', () => {
  it('parses a corpus at a path under backend/', () => {
    const corpus = loadBackendCorpus<{ cases: unknown[] }>(
      'internal/utils/testdata/dst_clock_corpus.json'
    )

    expect(corpus.cases.length).toBeGreaterThan(0)
  })

  it('names the resolved path of a file that is not there', () => {
    const missing = resolve(backendRoot, 'internal/utils/testdata/absent_corpus.json')

    expect(() =>
      loadBackendCorpus('internal/utils/testdata/absent_corpus.json')
    ).toThrow(`Cannot read backend corpus ${missing}`)
  })

  it('names the resolved path of a file that is not JSON', () => {
    expect(() => loadBackendCorpus('go.mod')).toThrow(
      `Cannot parse backend corpus ${resolve(backendRoot, 'go.mod')}`
    )
  })
})
