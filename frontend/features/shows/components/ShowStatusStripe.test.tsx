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
      'TONIGHT·DOORS 7PM·ENDS ~11PM (EST.)'
    )
  })

  it('renders nothing when the show has no readable date', () => {
    render(
      <ShowStatusStripe show={makeShow({ event_date: '' })} lifecycle="upcoming" />
    )
    expect(screen.queryByTestId('show-status-stripe')).not.toBeInTheDocument()
  })

  // The band is one row of type in every state, so nothing below it moves when
  // a show crosses from upcoming to tonight to past.
  it('keeps one band, one position, in every state', () => {
    for (const lifecycle of ['upcoming', 'today', 'past'] as const) {
      const { unmount } = render(
        <ShowStatusStripe show={makeShow()} lifecycle={lifecycle} />
      )
      expect(screen.getAllByTestId('show-status-stripe')).toHaveLength(1)
      unmount()
    }
  })
})
