import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  REPLY_PERMISSION_VALUES,
  REPLY_PERMISSION_LABELS,
  REPLY_PERMISSION_BADGE_LABELS,
  FIELD_NOTE_EDIT_WINDOW_MS,
  isWithinFieldNoteEditWindow,
} from './types'

describe('reply permission constants', () => {
  it('exposes the three permission values', () => {
    expect(REPLY_PERMISSION_VALUES).toEqual([
      'anyone',
      'followers',
      'author_only',
    ])
  })

  it('keeps a user-facing label for every permission value', () => {
    for (const value of REPLY_PERMISSION_VALUES) {
      expect(REPLY_PERMISSION_LABELS[value]).toBeTruthy()
    }
  })

  it('uses an empty badge for "anyone" (no badge when unrestricted)', () => {
    expect(REPLY_PERMISSION_BADGE_LABELS.anyone).toBe('')
  })

  it('keeps a non-empty badge for restricted permissions', () => {
    expect(REPLY_PERMISSION_BADGE_LABELS.followers).toBeTruthy()
    expect(REPLY_PERMISSION_BADGE_LABELS.author_only).toBeTruthy()
  })
})

describe('FIELD_NOTE_EDIT_WINDOW_MS', () => {
  it('is 30 minutes in milliseconds', () => {
    expect(FIELD_NOTE_EDIT_WINDOW_MS).toBe(30 * 60 * 1000)
  })
})

describe('isWithinFieldNoteEditWindow', () => {
  // The window is measured against Date.now(); pin the clock so the boundary
  // assertions are deterministic.
  const NOW = new Date('2026-05-19T12:00:00Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function createdMsAgo(ms: number): string {
    return new Date(NOW.getTime() - ms).toISOString()
  }

  it('returns true for a note created just now', () => {
    expect(isWithinFieldNoteEditWindow(createdMsAgo(0))).toBe(true)
  })

  it('returns true for a note created within the window', () => {
    expect(isWithinFieldNoteEditWindow(createdMsAgo(29 * 60 * 1000))).toBe(true)
  })

  it('returns false at exactly the window boundary (strict <)', () => {
    expect(
      isWithinFieldNoteEditWindow(createdMsAgo(FIELD_NOTE_EDIT_WINDOW_MS))
    ).toBe(false)
  })

  it('returns false for a note older than the window', () => {
    expect(isWithinFieldNoteEditWindow(createdMsAgo(60 * 60 * 1000))).toBe(
      false
    )
  })

  it('returns false for an unparseable timestamp', () => {
    expect(isWithinFieldNoteEditWindow('not-a-date')).toBe(false)
  })

  it('returns false for an empty string', () => {
    expect(isWithinFieldNoteEditWindow('')).toBe(false)
  })
})
