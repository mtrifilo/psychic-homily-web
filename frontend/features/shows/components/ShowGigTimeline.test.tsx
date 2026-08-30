import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowGigTimeline } from './ShowGigTimeline'
import type { ShowTimelineEntry } from '../types'

function makeEntry(overrides: Partial<ShowTimelineEntry> = {}): ShowTimelineEntry {
  return {
    show_id: 2,
    show_slug: 'metro-aug-9',
    // 9:00 PM Aug 9 in Chicago.
    event_date: '2025-08-10T02:00:00Z',
    timezone: 'America/Chicago',
    venue_name: 'Metro',
    venue_slug: 'metro',
    city: 'Chicago',
    state: 'IL',
    ...overrides,
  }
}

/**
 * The screen-reader prefix and the label are separate text nodes, and the
 * accessible name concatenates each node's trimmed text, so the gap between
 * them is whitespace-insensitive: `\s*`, not a literal space.
 */

/**
 * The show being read, as a stop on its own spine. The module formats it with
 * the same helpers it formats the neighbours with, so this is raw payload
 * rather than pre-built label strings: 9:00 PM Aug 12 in Chicago renders as
 * `AUG 12 SALT SHED, CHICAGO`, and its year is what the neighbours' dates are
 * compared against.
 */
const current = {
  event_date: '2025-08-13T02:00:00Z',
  timezone: 'America/Chicago',
  venue_name: 'Salt Shed',
  city: 'Chicago',
  state: 'IL',
}

describe('ShowGigTimeline', () => {
  it('renders nothing when there is no neighbour in either direction', () => {
    const { container } = render(
      <ShowGigTimeline current={current} previous={null} next={null} />
    )

    expect(container).toBeEmptyDOMElement()
    expect(screen.queryByTestId('show-gig-timeline')).not.toBeInTheDocument()
  })

  it('renders with only a previous neighbour', () => {
    render(
      <ShowGigTimeline current={current} previous={makeEntry()} next={null} />
    )

    expect(screen.getByTestId('show-gig-timeline')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /^Previous show:\s*AUG 9 METRO, CHICAGO$/ })
    ).toBeInTheDocument()
  })

  it('renders with only a next neighbour', () => {
    render(
      <ShowGigTimeline
        current={current}
        previous={null}
        next={makeEntry({
          show_slug: 'royal-oak-aug-14',
          event_date: '2025-08-15T02:00:00Z',
          venue_name: '',
          city: 'Royal Oak',
          state: 'MI',
        })}
      />
    )

    expect(screen.getByTestId('show-gig-timeline')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /^Next show:\s*AUG 14 ROYAL OAK, MI$/ })
    ).toBeInTheDocument()
  })

  it('links each neighbour to its own show page', () => {
    render(
      <ShowGigTimeline
        current={current}
        previous={makeEntry()}
        next={makeEntry({
          show_id: 3,
          show_slug: 'salt-shed-aug-14',
          event_date: '2025-08-15T02:00:00Z',
          venue_name: 'Salt Shed',
        })}
      />
    )

    expect(
      screen.getByRole('link', { name: /Previous show/ })
    ).toHaveAttribute('href', '/shows/metro-aug-9')
    expect(screen.getByRole('link', { name: /Next show/ })).toHaveAttribute(
      'href',
      '/shows/salt-shed-aug-14'
    )
  })

  // `/shows/` is the shows INDEX, so an unguarded href would silently take the
  // reader off the page instead of failing visibly.
  it('renders a slug-less neighbour as unlinked text', () => {
    const { container } = render(
      <ShowGigTimeline
        current={current}
        previous={makeEntry({ show_slug: '' })}
        next={null}
      />
    )

    expect(screen.queryAllByRole('link')).toHaveLength(0)
    expect(container.querySelector('a[href="/shows/"]')).toBeNull()
    expect(container.querySelector('a[href="/shows"]')).toBeNull()
    expect(container.textContent).toContain('AUG 9 METRO, CHICAGO')
  })

  it('renders the current date and place as text, never as a link', () => {
    render(
      <ShowGigTimeline current={current} previous={makeEntry()} next={null} />
    )

    const marker = screen.getByText('AUG 12 SALT SHED, CHICAGO')
    expect(marker.closest('a')).toBeNull()
    expect(
      screen.queryByRole('link', { name: /AUG 12 SALT SHED, CHICAGO/ })
    ).not.toBeInTheDocument()
  })

  // The arrows are aria-hidden decoration, so the direction has to be in the
  // announced text of each stop or it is not in the document at all.
  it('announces the direction of each stop to a screen reader', () => {
    render(
      <ShowGigTimeline
        current={current}
        previous={makeEntry()}
        next={makeEntry({
          show_id: 3,
          show_slug: 'salt-shed-aug-14',
          event_date: '2025-08-15T02:00:00Z',
          venue_name: 'Salt Shed',
        })}
      />
    )

    const [previous, next] = screen.getAllByRole('link')
    expect(previous).toHaveAccessibleName(/^Previous show:/)
    expect(next).toHaveAccessibleName(/^Next show:/)
  })

  // A neighbour in a different venue-local year carries its year; the subject
  // year is the one the spine compares against.
  it('carries the year on a neighbour outside the subject year', () => {
    render(
      <ShowGigTimeline
        current={current}
        previous={makeEntry({
          event_date: '2024-12-31T04:00:00Z',
          venue_name: 'Empty Bottle',
        })}
        next={null}
      />
    )

    expect(
      screen.getByRole('link', {
        name: /^Previous show:\s*DEC 30 2024 EMPTY BOTTLE, CHICAGO$/,
      })
    ).toBeInTheDocument()
  })
})
