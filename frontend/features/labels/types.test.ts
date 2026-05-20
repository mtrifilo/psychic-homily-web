import { describe, it, expect } from 'vitest'
import {
  LABEL_STATUS_LABELS,
  LABEL_STATUSES,
  getLabelStatusVariant,
  getLabelStatusLabel,
  formatLabelLocation,
  type LabelStatus,
} from './types'

describe('LABEL_STATUSES / LABEL_STATUS_LABELS', () => {
  it('lists the three known statuses', () => {
    expect(LABEL_STATUSES).toEqual(['active', 'inactive', 'defunct'])
  })

  it('has a display label for every status in the list', () => {
    for (const status of LABEL_STATUSES) {
      expect(LABEL_STATUS_LABELS[status]).toBeTruthy()
    }
  })

  it('maps statuses to title-cased display labels', () => {
    expect(LABEL_STATUS_LABELS.active).toBe('Active')
    expect(LABEL_STATUS_LABELS.inactive).toBe('Inactive')
    expect(LABEL_STATUS_LABELS.defunct).toBe('Defunct')
  })
})

describe('getLabelStatusVariant', () => {
  it('maps active to the default (filled) badge variant', () => {
    expect(getLabelStatusVariant('active')).toBe('default')
  })

  it('maps inactive to the secondary badge variant', () => {
    expect(getLabelStatusVariant('inactive')).toBe('secondary')
  })

  it('maps defunct to the outline badge variant', () => {
    expect(getLabelStatusVariant('defunct')).toBe('outline')
  })

  it('falls back to secondary for an unknown status', () => {
    expect(getLabelStatusVariant('mystery')).toBe('secondary')
  })
})

describe('getLabelStatusLabel', () => {
  it('returns the mapped label for a known status', () => {
    expect(getLabelStatusLabel('active')).toBe('Active')
    expect(getLabelStatusLabel('defunct')).toBe('Defunct')
  })

  it('title-cases an unknown status as a fallback', () => {
    expect(getLabelStatusLabel('hiatus')).toBe('Hiatus')
  })

  it('only capitalizes the first character of an unknown status', () => {
    // The fallback uppercases char[0] and appends the rest verbatim.
    expect(getLabelStatusLabel('on hold')).toBe('On hold')
  })

  it('handles a status that is a non-LabelStatus string narrowed at the call site', () => {
    const s: string = 'inactive'
    expect(getLabelStatusLabel(s as LabelStatus)).toBe('Inactive')
  })
})

describe('formatLabelLocation', () => {
  it('joins city and state with a comma when both are present', () => {
    expect(formatLabelLocation({ city: 'Seattle', state: 'WA' })).toBe(
      'Seattle, WA'
    )
  })

  it('returns just the city when state is null', () => {
    expect(formatLabelLocation({ city: 'Seattle', state: null })).toBe('Seattle')
  })

  it('returns just the state when city is null', () => {
    expect(formatLabelLocation({ city: null, state: 'WA' })).toBe('WA')
  })

  it('returns null when neither city nor state is present', () => {
    expect(formatLabelLocation({ city: null, state: null })).toBeNull()
  })
})
