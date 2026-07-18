import { describe, it, expect } from 'vitest'
import { truncatedCountPhrase, sentenceCase } from './truncatedCountPhrase'

const artists = { singular: 'artist', plural: 'artists' }
const items = { singular: 'item', plural: 'items' }

describe('truncatedCountPhrase (PSY-1476)', () => {
  it('renders the plain count, untruncated, when not truncated', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: 12, truncated: false, ...artists }),
    ).toEqual({ phrase: '12 artists', truncated: false })
  })

  it('singularizes a one-node count', () => {
    expect(
      truncatedCountPhrase({ shown: 1, total: 1, truncated: false, ...artists }).phrase,
    ).toBe('1 artist')
  })

  it('renders the top-N-of-M cue, truncated, when the total backs it up', () => {
    expect(
      truncatedCountPhrase({ shown: 150, total: 312, truncated: true, ...artists }),
    ).toEqual({ phrase: 'top 150 of 312 artists', truncated: true })
    expect(
      truncatedCountPhrase({ shown: 150, total: 312, truncated: true, ...items }),
    ).toEqual({ phrase: 'top 150 of 312 items', truncated: true })
  })

  it('degrades to the plain count when the flag is set but the total is nonsense', () => {
    // A skewed/stale payload must never render "top 12 of 0 items".
    for (const total of [0, 12, 5]) {
      expect(
        truncatedCountPhrase({ shown: 12, total, truncated: true, ...items }),
      ).toEqual({ phrase: '12 items', truncated: false })
    }
  })

  it('degrades to the plain count when the total is missing (older payload)', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: undefined, truncated: true, ...items }),
    ).toEqual({ phrase: '12 items', truncated: false })
  })

  it('never renders "top 0 of N" when nothing is shown', () => {
    // The collection backend sets nodes_truncated even when every node was
    // dropped (deleted-entity payload: 0 nodes, positive total).
    expect(
      truncatedCountPhrase({ shown: 0, total: 5, truncated: true, ...items }),
    ).toEqual({ phrase: '0 items', truncated: false })
  })

  it('reports not-truncated when the flag is off despite a present total', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: 312, truncated: false, ...items }),
    ).toEqual({ phrase: '12 items', truncated: false })
    expect(
      truncatedCountPhrase({ shown: 12, total: 312, truncated: undefined, ...items }).truncated,
    ).toBe(false)
  })
})

describe('sentenceCase (PSY-1476)', () => {
  it('capitalizes a leading letter', () => {
    expect(sentenceCase('top 150 of 312 items')).toBe('Top 150 of 312 items')
  })

  it('is a no-op for a digit-leading plain count', () => {
    expect(sentenceCase('12 artists')).toBe('12 artists')
  })
})
