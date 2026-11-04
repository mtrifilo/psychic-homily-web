import { describe, it, expect } from 'vitest'
import { episodePlayHref, parsePlayAnchor, playAnchorId } from './playAnchor'

describe('play anchors', () => {
  it('round-trips a play id through the href and the hash', () => {
    const href = episodePlayHref('kexp', 'morning-show', '2026-11-02', 512)
    expect(href).toBe('/radio/kexp/morning-show/2026-11-02#play-512')
    expect(parsePlayAnchor(href.slice(href.indexOf('#')))).toBe(512)
    expect(`#${playAnchorId(512)}`).toBe('#play-512')
  })

  it.each(['', '#', '#play-', '#play-abc', '#play-12x', '#track-12', 'play-12', '#play--1'])(
    'reads %j as no anchor',
    hash => {
      expect(parsePlayAnchor(hash)).toBeNull()
    }
  )

  it('rejects an id past the safe integer range', () => {
    expect(parsePlayAnchor('#play-99999999999999999999')).toBeNull()
  })
})
