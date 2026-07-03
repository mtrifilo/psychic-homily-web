import { describe, it, expect } from 'vitest'
import { showDisplayTitle, UNTITLED_SHOW } from './showDisplayTitle'

describe('showDisplayTitle (PSY-1328)', () => {
  it('prefers the title, trimmed', () => {
    expect(showDisplayTitle('  Night of Noise ', ['Band A'])).toBe('Night of Noise')
  })

  it('treats a whitespace-only title as missing (the invisible-link bug)', () => {
    expect(showDisplayTitle('   ', ['Band A'])).toBe('Band A')
  })

  it('joins bill names when there is no title', () => {
    expect(showDisplayTitle(null, ['Band A', 'Band B'])).toBe('Band A, Band B')
  })

  it('filters blank bill entries before joining (a [""] payload is not a bill)', () => {
    expect(showDisplayTitle('', ['', '  ', 'Band A'])).toBe('Band A')
    expect(showDisplayTitle('', ['', '  '])).toBe(UNTITLED_SHOW)
  })

  it('caps the bill with "+N more"', () => {
    expect(showDisplayTitle(null, ['A', 'B', 'C', 'D', 'E'], { cap: 3 })).toBe(
      'A, B, C +2 more',
    )
  })

  it('renders no "+0 more" exactly at the cap', () => {
    expect(showDisplayTitle(null, ['A', 'B', 'C'], { cap: 3 })).toBe('A, B, C')
  })

  it('composes the venue into the fallback only, never into a real title', () => {
    expect(showDisplayTitle(null, ['Band A'], { venueName: 'The Spot' })).toBe(
      'Band A @ The Spot',
    )
    expect(showDisplayTitle(null, [], { venueName: 'The Spot' })).toBe(
      'Show @ The Spot',
    )
    expect(showDisplayTitle('Real Title', ['Band A'], { venueName: 'The Spot' })).toBe(
      'Real Title',
    )
  })

  it('caps and composes venue together', () => {
    expect(
      showDisplayTitle(null, ['A', 'B', 'C', 'D'], { cap: 2, venueName: 'The Spot' }),
    ).toBe('A, B +2 more @ The Spot')
  })

  it('falls back to the one consistent empty state', () => {
    expect(showDisplayTitle(null, null)).toBe(UNTITLED_SHOW)
    expect(showDisplayTitle(undefined, undefined)).toBe(UNTITLED_SHOW)
    expect(showDisplayTitle('', [])).toBe(UNTITLED_SHOW)
  })

  it('tolerates null entries inside the names array (omitempty trust boundary)', () => {
    expect(showDisplayTitle(null, [null, undefined, 'Band A'])).toBe('Band A')
  })

  it('ignores a non-positive cap (defensive)', () => {
    expect(showDisplayTitle(null, ['A', 'B'], { cap: 0 })).toBe('A, B')
  })
})
