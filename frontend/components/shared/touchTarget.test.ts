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

  it('makes the hit area at least 24px on both axes', () => {
    expect(tokens).toEqual(
      expect.arrayContaining([
        'pointer-coarse:after:absolute',
        'pointer-coarse:after:min-h-6',
        'pointer-coarse:after:min-w-6',
      ])
    )
  })

  it('paints nothing', () => {
    const painting = tokens.filter(token =>
      /:(bg|border|shadow|ring|outline|text|opacity)-/.test(token)
    )
    expect(painting).toEqual([])
  })
})
