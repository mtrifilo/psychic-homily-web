import { describe, it, expect } from 'vitest'
import {
  FILTER_SELECT_ALL,
  toFilterSelectValue,
  fromFilterSelectValue,
} from './filterSelectValue'
import { RELEASE_TYPES } from '@/features/releases/types'
import { LABEL_STATUSES } from '@/features/labels/types'
import { FESTIVAL_STATUSES } from '@/features/festivals/types'
import { TAG_CATEGORIES } from '@/features/tags/types'

describe('filter Select "All" sentinel mapping (PSY-924)', () => {
  it('maps the empty (no filter) state to the Radix sentinel', () => {
    expect(toFilterSelectValue('')).toBe(FILTER_SELECT_ALL)
  })

  it('passes a real filter value through to the Select unchanged', () => {
    expect(toFilterSelectValue('lp')).toBe('lp')
  })

  it('maps the sentinel back to the empty (no filter) state', () => {
    // Guards the regression where state would persist the literal 'all' and
    // send an invalid filter value to the backend instead of clearing it.
    expect(fromFilterSelectValue(FILTER_SELECT_ALL)).toBe('')
  })

  it('passes a real selected value back to state unchanged', () => {
    expect(fromFilterSelectValue('active')).toBe('active')
  })

  it('round-trips the empty state and every entity filter value', () => {
    const values = [
      '',
      ...RELEASE_TYPES,
      ...LABEL_STATUSES,
      ...FESTIVAL_STATUSES,
      ...TAG_CATEGORIES,
    ]
    for (const value of values) {
      expect(fromFilterSelectValue(toFilterSelectValue(value))).toBe(value)
    }
  })

  it('uses a sentinel that cannot collide with any real filter value', () => {
    // Asserts against the live filter-value lists so a future enum addition
    // named 'all' fails here instead of silently shipping a filter that can
    // never be selected (its SelectItem value would equal the "All" item).
    const realValues = [
      ...RELEASE_TYPES,
      ...LABEL_STATUSES,
      ...FESTIVAL_STATUSES,
      ...TAG_CATEGORIES,
    ] as string[]
    expect(realValues).not.toContain(FILTER_SELECT_ALL)
  })
})
