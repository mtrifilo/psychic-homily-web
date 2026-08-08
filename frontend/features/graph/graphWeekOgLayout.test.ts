import { describe, expect, it } from 'vitest'

import { OG_SIZE } from '@/lib/og/brand'
import { measureMono } from '@/lib/og/textFit'

import {
  CONTENT_WIDTH,
  COUNTS_MAX_WIDTH,
  COUNTS_SIZE_MAX,
  COUNTS_SIZE_MIN,
  COUNTS_TRACKING,
  EYEBROW_SIZE,
  MOTIF_FADE_CLEAR_STOP,
  PAD_X,
  RANGE_SIZE,
  RANGE_TRACKING,
  TEXT_WIDTH,
  eyebrowWidth,
  fitCountsSize,
  headlineLongestWordWidth,
} from './graphWeekOgLayout'

/**
 * The family's rule: design at full size, verify at 300px — a link renders about
 * that wide in a group chat — and treat anything under ~8px effective as
 * decoration that must not carry meaning. Every line on this card carries
 * meaning, so every line has to clear the floor.
 */
const SHARE_DOWNSCALE = 4
const LEGIBILITY_FLOOR_PX = 8

describe('card type budgets', () => {
  it('keeps every line above the 300px legibility floor', () => {
    for (const [label, size] of [
      ['eyebrow', EYEBROW_SIZE],
      ['range', RANGE_SIZE],
      ['counts at their floor', COUNTS_SIZE_MIN],
    ] as const) {
      expect(size / SHARE_DOWNSCALE, label).toBeGreaterThanOrEqual(LEGIBILITY_FLOOR_PX)
    }
  })

  it('fits the eyebrow inside the content box at its fixed size', () => {
    // It is deliberately given the FULL content width rather than the text
    // column's, which is what lets it hold 34px. If this ever fails, the copy or
    // the width changed — not the size.
    expect(eyebrowWidth()).toBeLessThanOrEqual(CONTENT_WIDTH)
  })

  it('fits the headline inside the text column without breaking a word', () => {
    // The headline has no fit function because its copy is a constant. This is
    // the measurement that stands in for one: the longest single word must fit,
    // or the wrap would clip mid-word instead of at a space.
    expect(headlineLongestWordWidth()).toBeLessThanOrEqual(TEXT_WIDTH)
  })

  it('fits the range on one line', () => {
    expect(
      measureMono('DEC 28 2025 - JAN 3 2026', RANGE_SIZE, RANGE_TRACKING)
    ).toBeLessThanOrEqual(TEXT_WIDTH)
  })
})

describe('fitCountsSize', () => {
  it('uses the full size for an ordinary week', () => {
    const ordinary = '+12 ARTISTS · +34 CONNECTIONS'
    expect(fitCountsSize(ordinary)).toBe(COUNTS_SIZE_MAX)
    expect(measureMono(ordinary, COUNTS_SIZE_MAX, COUNTS_TRACKING)).toBeLessThanOrEqual(
      COUNTS_MAX_WIDTH
    )
  })

  it('seats the widest line this card can produce without overrunning', () => {
    // This is the case that fixes COUNTS_MAX_WIDTH. Five figures on both halves
    // is a mass-import week, not a normal one, but the budget has to hold it —
    // the count is the whole assertion of the card and must not clip.
    const widest = '+9,999 ARTISTS · +99,999 CONNECTIONS'
    const size = fitCountsSize(widest)
    expect(size).toBeLessThan(COUNTS_SIZE_MAX)
    expect(size).toBeGreaterThanOrEqual(COUNTS_SIZE_MIN)
    expect(measureMono(widest, size, COUNTS_TRACKING)).toBeLessThanOrEqual(COUNTS_MAX_WIDTH)
  })

  it('never returns below the legibility floor even for absurd input', () => {
    const absurd = '+1 ARTIST · +2 CONNECTIONS'.repeat(6)
    expect(fitCountsSize(absurd)).toBe(COUNTS_SIZE_MIN)
    // Past the floor the line is over its budget by design — but it must still
    // CLIP inside the canvas rather than bleed off it. `nowrap` plus a text
    // column narrower than the content box is what makes that true.
    expect(measureMono(absurd, COUNTS_SIZE_MIN, COUNTS_TRACKING)).toBeGreaterThan(
      COUNTS_MAX_WIDTH
    )
  })

  it('keeps the counts line inside the motif fade at every size it can pick', () => {
    // The line is set over the gradient, and past MOTIF_FADE_CLEAR_STOP there is
    // no gradient left to sit on. Checked against the widest realistic line at
    // the size the fit function actually returns for it.
    const clearStopPx = (OG_SIZE.width * MOTIF_FADE_CLEAR_STOP) / 100
    const widest = '+9,999 ARTISTS · +99,999 CONNECTIONS'
    const right = PAD_X + measureMono(widest, fitCountsSize(widest), COUNTS_TRACKING)
    expect(right).toBeLessThanOrEqual(clearStopPx)
  })
})
