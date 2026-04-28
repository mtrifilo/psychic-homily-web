import { describe, it, expect } from 'vitest'
import { buildLinkLabel } from './ArtistGraph'

// PSY-362: tooltip strings are the user-facing surface of the underlying signal.
// These tests pin the format per edge type so future detail-shape changes don't
// silently drop information from the tooltip.
describe('buildLinkLabel (PSY-362 edge tooltip text)', () => {
  describe('similar', () => {
    it('formats score as percent with vote totals when votes exist', () => {
      expect(
        buildLinkLabel({
          type: 'similar',
          score: 0.85,
          votes_up: 8,
          votes_down: 2,
          detail: undefined,
        })
      ).toBe('Similar: 85% (8 up / 2 down)')
    })

    it('omits vote totals when there are no votes (auto-derived only)', () => {
      expect(
        buildLinkLabel({
          type: 'similar',
          score: 0.5,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Similar: 50%')
    })

    it('rounds non-integer score percentages', () => {
      expect(
        buildLinkLabel({
          type: 'similar',
          score: 0.6789,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Similar: 68%')
    })
  })

  describe('shared_bills', () => {
    it('reports count + last-shared date when both are in detail', () => {
      expect(
        buildLinkLabel({
          type: 'shared_bills',
          score: 0.4,
          votes_up: 0,
          votes_down: 0,
          detail: { shared_count: 7, last_shared: '2026-03-01' },
        })
      ).toBe('7 shared shows (last: 2026-03-01)')
    })

    it('singularizes when count is 1', () => {
      expect(
        buildLinkLabel({
          type: 'shared_bills',
          score: 0.1,
          votes_up: 0,
          votes_down: 0,
          detail: { shared_count: 1, last_shared: '2026-03-01' },
        })
      ).toBe('1 shared show (last: 2026-03-01)')
    })

    it('omits date when missing from detail', () => {
      expect(
        buildLinkLabel({
          type: 'shared_bills',
          score: 0.4,
          votes_up: 0,
          votes_down: 0,
          detail: { shared_count: 4 },
        })
      ).toBe('4 shared shows')
    })

    it('falls back when detail is missing entirely', () => {
      expect(
        buildLinkLabel({
          type: 'shared_bills',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Shared bills')
    })
  })

  describe('shared_label', () => {
    it('shows "Both on {label}" for a single shared label', () => {
      expect(
        buildLinkLabel({
          type: 'shared_label',
          score: 0.2,
          votes_up: 0,
          votes_down: 0,
          detail: { shared_count: 1, label_names: 'Closed Casket Activities' },
        })
      ).toBe('Both on Closed Casket Activities')
    })

    it('lists labels when multiple are shared', () => {
      expect(
        buildLinkLabel({
          type: 'shared_label',
          score: 0.4,
          votes_up: 0,
          votes_down: 0,
          detail: {
            shared_count: 2,
            label_names: 'Closed Casket Activities, Profound Lore',
          },
        })
      ).toBe('2 shared labels: Closed Casket Activities, Profound Lore')
    })

    it('falls back to count-only when label names absent', () => {
      expect(
        buildLinkLabel({
          type: 'shared_label',
          score: 0.4,
          votes_up: 0,
          votes_down: 0,
          detail: { shared_count: 3 },
        })
      ).toBe('3 shared labels')
    })

    it('uses generic fallback when detail is empty', () => {
      expect(
        buildLinkLabel({
          type: 'shared_label',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Shared label')
    })
  })

  describe('radio_cooccurrence', () => {
    it('reports count and station breakdown when stations > 1', () => {
      expect(
        buildLinkLabel({
          type: 'radio_cooccurrence',
          score: 0.6,
          votes_up: 0,
          votes_down: 0,
          detail: { co_occurrence_count: 14, station_count: 3, show_count: 9 },
        })
      ).toBe('Played together on 14 radio shows across 3 stations')
    })

    it('omits station breakdown when only 1 station', () => {
      expect(
        buildLinkLabel({
          type: 'radio_cooccurrence',
          score: 0.3,
          votes_up: 0,
          votes_down: 0,
          detail: { co_occurrence_count: 5, station_count: 1 },
        })
      ).toBe('Played together on 5 radio shows')
    })

    it('singularizes when count is 1', () => {
      expect(
        buildLinkLabel({
          type: 'radio_cooccurrence',
          score: 0.1,
          votes_up: 0,
          votes_down: 0,
          detail: { co_occurrence_count: 1, station_count: 1 },
        })
      ).toBe('Played together on 1 radio show')
    })

    it('falls back when detail is missing', () => {
      expect(
        buildLinkLabel({
          type: 'radio_cooccurrence',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Radio co-occurrence')
    })
  })

  describe('side_project / member_of (binary facts)', () => {
    it('side_project tooltip is descriptive without magnitude', () => {
      expect(
        buildLinkLabel({
          type: 'side_project',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Side project')
    })

    it('member_of tooltip is descriptive without magnitude', () => {
      expect(
        buildLinkLabel({
          type: 'member_of',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('Member of')
    })
  })

  describe('unknown edge types', () => {
    it('falls back to the type string when not recognised', () => {
      expect(
        buildLinkLabel({
          type: 'totally_made_up',
          score: 0,
          votes_up: 0,
          votes_down: 0,
          detail: undefined,
        })
      ).toBe('totally_made_up')
    })
  })
})

// PSY-363: festival_cobill tooltip variants. The helper needs to gracefully
// degrade as fields drop out of the loosely-typed `detail` JSONB.
describe('buildLinkLabel — festival_cobill (PSY-363)', () => {
  it('shows count, names, and last year when all fields populate', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: {
          festival_names: 'ACL, Coachella, Lollapalooza',
          count: 3,
          most_recent_year: 2025,
        },
      })
    ).toBe('3 shared festivals: ACL, Coachella, Lollapalooza (last: 2025)')
  })

  it('falls back to count + last year when names are sparse', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: {
          festival_names: '',
          count: 3,
          most_recent_year: 2025,
        },
      })
    ).toBe('3 shared festivals (last: 2025)')
  })

  it('falls back to count only when both names and year are missing', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: { count: 3 },
      })
    ).toBe('3 shared festivals')
  })

  it('uses singular noun when count is 1', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: {
          festival_names: 'Coachella',
          count: 1,
          most_recent_year: 2025,
        },
      })
    ).toBe('1 shared festival: Coachella (last: 2025)')
  })

  it('falls back to the static label when detail is missing entirely', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: undefined,
      })
    ).toBe('Festival co-lineup')
  })

  it('falls back to the static label when count is missing', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: { festival_names: 'Coachella' },
      })
    ).toBe('Festival co-lineup')
  })

  it('coerces a string count to a number', () => {
    expect(
      buildLinkLabel({
        type: 'festival_cobill',
        score: 0,
        votes_up: 0,
        votes_down: 0,
        detail: { festival_names: 'Coachella', count: '2', most_recent_year: '2024' },
      })
    ).toBe('2 shared festivals: Coachella (last: 2024)')
  })
})
