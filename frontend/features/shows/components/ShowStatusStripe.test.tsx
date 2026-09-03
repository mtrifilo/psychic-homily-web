import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ShowStatusStripe } from './ShowStatusStripe'
import type { ShowResponse } from '../types'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    // 8 PM Wed Apr 15 2026 in Phoenix.
    event_date: '2026-04-16T03:00:00Z',
    status: 'approved',
    state: 'AZ',
    venues: [
      {
        id: 1,
        slug: 'the-venue',
        name: 'The Venue',
        city: 'Phoenix',
        state: 'AZ',
        timezone: 'America/Phoenix',
        verified: true,
      },
    ],
    artists: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowStatusStripe', () => {
  it('joins its segments with a middot', () => {
    render(
      <ShowStatusStripe
        show={makeShow({ doors_at: '2026-04-16T02:00:00Z' })}
        lifecycle="today"
      />
    )
    expect(screen.getByTestId('show-status-stripe').textContent).toBe(
      'TONIGHT· DOORS 7PM· ENDS ~11PM'
    )
  })

  // The middot is decorative and hidden from assistive tech, so the spacing
  // around it is the only thing keeping the segments from being announced as
  // one run-on word ("TONIGHTDOORS 7PM").
  it('leaves a word boundary between segments for a screen reader', () => {
    render(
      <ShowStatusStripe
        show={makeShow({ doors_at: '2026-04-16T02:00:00Z' })}
        lifecycle="today"
      />
    )
    const stripe = screen.getByTestId('show-status-stripe')
    const spoken = Array.from(stripe.querySelectorAll('[aria-hidden="true"]'))
      .reduce(
        (text, hidden) => text.replace(hidden.textContent ?? '', ''),
        stripe.textContent ?? ''
      )
      .replace(/\s+/g, ' ')
      .trim()
    expect(spoken).toBe('TONIGHT DOORS 7PM ENDS ~11PM')
  })

  // An admin marking a show cancelled rewrites this band in place, with no
  // navigation, so assistive tech needs it to be a live region.
  it('is a polite live region so an in-place state change is announced', () => {
    render(<ShowStatusStripe show={makeShow()} lifecycle="upcoming" />)
    const stripe = screen.getByTestId('show-status-stripe')
    expect(stripe).toHaveAttribute('role', 'status')
    expect(stripe).toHaveAttribute('aria-live', 'polite')
  })

  it('renders nothing when the show has no readable date', () => {
    render(
      <ShowStatusStripe show={makeShow({ event_date: '' })} lifecycle="upcoming" />
    )
    expect(screen.queryByTestId('show-status-stripe')).not.toBeInTheDocument()
  })

  // TONAL, not inverted: the band tints itself out of the page's own palette
  // and bounds itself with hairline rules rather than reversing the page's
  // contrast. jsdom computes no colors, so the class names are the mechanism
  // and what is pinned; the negatives pin that the inversion is gone, which an
  // element carrying both sets of classes would otherwise satisfy.
  it('paints the band and its separators with the surface tokens', () => {
    render(
      <ShowStatusStripe
        show={makeShow({ doors_at: '2026-04-16T02:00:00Z' })}
        lifecycle="today"
      />
    )
    const stripe = screen.getByTestId('show-status-stripe')
    expect(stripe).toHaveClass(
      'bg-muted',
      'text-foreground',
      'border-y',
      'border-border'
    )
    expect(stripe).not.toHaveClass('bg-foreground')
    expect(stripe).not.toHaveClass('text-background')

    const separators = screen.getAllByText('\u00b7')
    expect(separators.length).toBeGreaterThan(0)
    for (const separator of separators) {
      expect(separator).toHaveClass('text-muted-foreground')
    }
  })

  // The band is one row of type in every state, reserving the same height, so
  // nothing below it moves when a show crosses from upcoming to tonight to
  // past. `min-h-11` is the mechanism; assert it rather than the rendered
  // height, which jsdom does not compute.
  it('reserves the same height in every state', () => {
    for (const lifecycle of ['upcoming', 'today', 'past'] as const) {
      const { unmount } = render(
        <ShowStatusStripe show={makeShow()} lifecycle={lifecycle} />
      )
      const bands = screen.getAllByTestId('show-status-stripe')
      expect(bands).toHaveLength(1)
      expect(bands[0].firstElementChild).toHaveClass('min-h-11')
      unmount()
    }
  })
})
