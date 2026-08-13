import { describe, it, expect } from 'vitest'
import { sceneDetailOgImageUrl, sceneDetailOgImages } from './sceneDetailShare'

describe('sceneDetailOgImageUrl', () => {
  it('points at the archived week card, whose URL carries the week', () => {
    expect(sceneDetailOgImageUrl('phoenix-az', '2026-W33')).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W33/opengraph-image'
    )
  })
})

describe('sceneDetailOgImages', () => {
  it('supplies width, height, type and alt so the file convention is not needed', () => {
    expect(sceneDetailOgImages('phoenix-az', '2026-W33', 'Phoenix this week')).toEqual([
      {
        url: 'https://psychichomily.com/scenes/phoenix-az/2026-W33/opengraph-image',
        width: 1200,
        height: 630,
        type: 'image/png',
        alt: 'Phoenix this week',
      },
    ])
  })
})
