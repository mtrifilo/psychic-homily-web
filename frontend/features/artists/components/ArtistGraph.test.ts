import { describe, it, expect } from 'vitest'
import { buildLinkLabel } from './ArtistGraph'

// PSY-363: hover-tooltip text for the festival_cobill edge type. The
// helper needs to gracefully degrade as fields drop out of the loosely-
// typed `detail` JSONB.

describe('buildLinkLabel — festival_cobill', () => {
  it('shows count, names, and last year when all fields populate', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: {
        festival_names: 'ACL, Coachella, Lollapalooza',
        count: 3,
        most_recent_year: 2025,
      },
    })
    expect(label).toBe('3 shared festivals: ACL, Coachella, Lollapalooza (last: 2025)')
  })

  it('falls back to count + last year when names are sparse', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: {
        festival_names: '',
        count: 3,
        most_recent_year: 2025,
      },
    })
    expect(label).toBe('3 shared festivals (last: 2025)')
  })

  it('falls back to count only when both names and year are missing', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: {
        count: 3,
      },
    })
    expect(label).toBe('3 shared festivals')
  })

  it('uses singular noun when count is 1', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: {
        festival_names: 'Coachella',
        count: 1,
        most_recent_year: 2025,
      },
    })
    expect(label).toBe('1 shared festival: Coachella (last: 2025)')
  })

  it('falls back to the static label when detail is missing entirely', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: undefined,
    })
    expect(label).toBe('Festival co-lineup')
  })

  it('falls back to the static label when count is missing', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: { festival_names: 'Coachella' },
    })
    expect(label).toBe('Festival co-lineup')
  })

  it('coerces a string count to a number', () => {
    const label = buildLinkLabel({
      type: 'festival_cobill',
      detail: { festival_names: 'Coachella', count: '2', most_recent_year: '2024' },
    })
    expect(label).toBe('2 shared festivals: Coachella (last: 2024)')
  })
})

describe('buildLinkLabel — fallback for other edge types', () => {
  it('returns the static EDGE_LABELS entry for known types', () => {
    expect(buildLinkLabel({ type: 'similar', detail: {} })).toBe('Similar')
    expect(buildLinkLabel({ type: 'shared_bills', detail: {} })).toBe('Shared Bills')
    expect(buildLinkLabel({ type: 'shared_label', detail: {} })).toBe('Shared Label')
    expect(buildLinkLabel({ type: 'side_project', detail: {} })).toBe('Side Project')
    expect(buildLinkLabel({ type: 'member_of', detail: {} })).toBe('Member Of')
    expect(buildLinkLabel({ type: 'radio_cooccurrence', detail: {} })).toBe('Radio Co-occurrence')
  })

  it('returns the raw type string when nothing is registered', () => {
    expect(buildLinkLabel({ type: 'unknown_type', detail: {} })).toBe('unknown_type')
  })
})
