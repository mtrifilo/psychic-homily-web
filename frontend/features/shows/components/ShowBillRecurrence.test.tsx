import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowBillRecurrence } from './ShowBillRecurrence'
import type { ShowTimelineEntry, ShowTimelineRecurrence } from '../types'

const artists = [
  { id: 1, name: 'Modest Mouse' },
  { id: 2, name: 'Califone' },
]

function makeStop(overrides: Partial<ShowTimelineEntry> = {}): ShowTimelineEntry {
  return {
    show_id: 9,
    show_slug: 'aragon-nov-2023',
    // 8:00 PM Nov 14 2023 in Chicago.
    event_date: '2023-11-15T02:00:00Z',
    timezone: 'America/Chicago',
    venue_name: 'Aragon Ballroom',
    venue_slug: 'aragon-ballroom',
    city: 'Chicago',
    state: 'IL',
    ...overrides,
  }
}

function makeEntry(
  overrides: Partial<ShowTimelineRecurrence> = {}
): ShowTimelineRecurrence {
  return {
    artist_id: 1,
    is_hometown: false,
    last_played: makeStop(),
    ...overrides,
  }
}

/** The rendered line as one string, which is how the reader receives it. */
function lineText(): string {
  return screen.getByTestId('show-bill-recurrence').textContent ?? ''
}

describe('ShowBillRecurrence', () => {
  it('renders nothing when the archive has nothing to say', () => {
    const { container } = render(
      <ShowBillRecurrence recurrence={[]} artists={artists} city="Chicago" />
    )

    expect(container).toBeEmptyDOMElement()
    expect(screen.queryByTestId('show-bill-recurrence')).not.toBeInTheDocument()
  })

  it('states when and where an act last played this city', () => {
    render(
      <ShowBillRecurrence
        recurrence={[makeEntry()]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(lineText()).toBe(
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom'
    )
  })

  // "Califone last played Chicago" is true of every Chicago band and says
  // nothing; living here is the fact worth stating.
  it('states the hometown claim instead of a prior date when both are known', () => {
    render(
      <ShowBillRecurrence
        recurrence={[
          makeEntry({ artist_id: 2, is_hometown: true, last_played: makeStop() }),
        ]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(lineText()).toBe('Califone: hometown show')
    expect(lineText()).not.toContain('last played')
  })

  it('states the hometown claim for an act with no prior date on record', () => {
    render(
      <ShowBillRecurrence
        recurrence={[
          makeEntry({ artist_id: 2, is_hometown: true, last_played: null }),
        ]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(lineText()).toBe('Califone: hometown show')
  })

  it('joins two acts with a middot and names both', () => {
    render(
      <ShowBillRecurrence
        recurrence={[
          makeEntry(),
          makeEntry({ artist_id: 2, is_hometown: true, last_played: null }),
        ]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(lineText()).toBe(
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom · Califone: hometown show'
    )
  })

  // Names come from the bill, so an entry the bill cannot name is dropped
  // rather than rendered nameless or left holding a separator.
  it('drops an entry for an act that is not on the bill', () => {
    render(
      <ShowBillRecurrence
        recurrence={[
          makeEntry({ artist_id: 404 }),
          makeEntry(),
          makeEntry({ artist_id: 405, is_hometown: true }),
        ]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(lineText()).toBe(
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom'
    )
    expect(lineText()).not.toContain('·')
  })

  it('renders nothing when no entry names an act on the bill', () => {
    const { container } = render(
      <ShowBillRecurrence
        recurrence={[makeEntry({ artist_id: 404 })]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(container).toBeEmptyDOMElement()
  })

  // An act with neither a hometown claim nor a prior date contributes no
  // segment at all.
  it('drops an entry with no hometown claim and no prior date', () => {
    const { container } = render(
      <ShowBillRecurrence
        recurrence={[makeEntry({ is_hometown: false, last_played: null })]}
        artists={artists}
        city="Chicago"
      />
    )

    expect(container).toBeEmptyDOMElement()
  })

  it('drops the city clause without a dangling space for a venue-less show', () => {
    render(
      <ShowBillRecurrence recurrence={[makeEntry()]} artists={artists} city="" />
    )

    expect(lineText()).toBe(
      'Modest Mouse last played: Nov 2023, Aragon Ballroom'
    )
  })
})
