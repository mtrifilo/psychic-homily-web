import { describe, it, expect } from 'vitest'
import { COARSE_POINTER_HIT_AREA_CLASS } from './touchTarget'

const tokens = COARSE_POINTER_HIT_AREA_CLASS.split(' ')

describe('COARSE_POINTER_HIT_AREA_CLASS', () => {
  it('applies nothing on a fine pointer', () => {
    for (const token of tokens) {
      expect(token.startsWith('pointer-coarse:')).toBe(true)
    }
  })

  // Anything on the host itself other than establishing the containing block
  // would move or resize what the reader sees.
  it('sizes only the pseudo-element, leaving the host box alone', () => {
    const hostTokens = tokens.filter(
      token => !token.startsWith('pointer-coarse:after:')
    )
    expect(hostTokens).toEqual(['pointer-coarse:relative'])
  })

  // The full set, so that dropping a centring or sizing token is a deliberate
  // edit here: without the translate the box hangs off one corner and covers
  // the neighbouring chip, and nothing in jsdom renders the CSS to notice.
  it('is exactly a centred box of at least 24px that paints nothing', () => {
    expect([...tokens].sort()).toEqual(
      [
        'pointer-coarse:relative',
        'pointer-coarse:after:absolute',
        'pointer-coarse:after:top-1/2',
        'pointer-coarse:after:left-1/2',
        'pointer-coarse:after:size-full',
        'pointer-coarse:after:min-h-6',
        'pointer-coarse:after:min-w-6',
        'pointer-coarse:after:-translate-1/2',
      ].sort()
    )
  })
})
