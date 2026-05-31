import { describe, it, expect } from 'vitest'
import {
  PLAYLIST_SOURCES,
  PLAYLIST_SOURCE_NONE,
  toPlaylistSelectValue,
  fromPlaylistSelectValue,
} from './playlistSourceSelect'

describe('playlist-source Select sentinel mapping', () => {
  it('maps the empty (no source) state to the Radix sentinel', () => {
    expect(toPlaylistSelectValue('')).toBe(PLAYLIST_SOURCE_NONE)
  })

  it('passes a real source value through to the Select unchanged', () => {
    expect(toPlaylistSelectValue('kexp_api')).toBe('kexp_api')
  })

  it('maps the sentinel back to the empty (no source) state', () => {
    // Guards the regression where state would persist the literal 'none'
    // and submit an invalid playlist_source to the backend.
    expect(fromPlaylistSelectValue(PLAYLIST_SOURCE_NONE)).toBe('')
  })

  it('passes a real selected value back to state unchanged', () => {
    expect(fromPlaylistSelectValue('wfmu_scrape')).toBe('wfmu_scrape')
  })

  it('round-trips every state through the Select and back', () => {
    for (const source of ['', 'kexp_api', 'wfmu_scrape', 'nts_api', 'manual']) {
      expect(fromPlaylistSelectValue(toPlaylistSelectValue(source))).toBe(source)
    }
  })

  it('uses a sentinel that cannot collide with a real source value', () => {
    // 'none' is not one of the PLAYLIST_SOURCES values, so a real selection
    // can never be misread as "no source". Asserts against the live list so a
    // future source addition named 'none' would fail here instead of silently
    // shipping the collision.
    const realSources = PLAYLIST_SOURCES.map((s) => s.value)
    expect(realSources).not.toContain(PLAYLIST_SOURCE_NONE)
  })
})
