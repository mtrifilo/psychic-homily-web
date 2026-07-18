import { describe, it, expect } from 'vitest'
import { truncatedCountPhrase } from './truncatedCountPhrase'

const artists = { singular: 'artist', plural: 'artists' }
const items = { singular: 'item', plural: 'items' }

describe('truncatedCountPhrase (PSY-1476)', () => {
  it('renders the plain count when not truncated', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: 12, truncated: false, ...artists }),
    ).toBe('12 artists')
  })

  it('singularizes a one-node count', () => {
    expect(
      truncatedCountPhrase({ shown: 1, total: 1, truncated: false, ...artists }),
    ).toBe('1 artist')
  })

  it('renders the top-N-of-M cue when truncated and the total backs it up', () => {
    expect(
      truncatedCountPhrase({ shown: 150, total: 312, truncated: true, ...artists }),
    ).toBe('top 150 of 312 artists')
    expect(
      truncatedCountPhrase({ shown: 150, total: 312, truncated: true, ...items }),
    ).toBe('top 150 of 312 items')
  })

  it('degrades to the plain count when the flag is set but the total is nonsense', () => {
    // A skewed/stale payload must never render "top 12 of 0 items".
    expect(
      truncatedCountPhrase({ shown: 12, total: 0, truncated: true, ...items }),
    ).toBe('12 items')
    expect(
      truncatedCountPhrase({ shown: 12, total: 12, truncated: true, ...items }),
    ).toBe('12 items')
    // total ≤ shown is also nonsense (the "top" would be a superset claim).
    expect(
      truncatedCountPhrase({ shown: 12, total: 5, truncated: true, ...items }),
    ).toBe('12 items')
  })

  it('degrades to the plain count when the total is missing (older payload)', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: undefined, truncated: true, ...items }),
    ).toBe('12 items')
  })

  it('renders the plain count when the total is present but the flag is off', () => {
    expect(
      truncatedCountPhrase({ shown: 12, total: 312, truncated: false, ...items }),
    ).toBe('12 items')
    expect(
      truncatedCountPhrase({ shown: 12, total: 312, truncated: undefined, ...items }),
    ).toBe('12 items')
  })
})
