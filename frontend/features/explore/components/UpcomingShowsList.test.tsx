import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UpcomingShowsList } from './UpcomingShowsList'
import type { ExploreUpcomingShowsResponse } from '../types'

type MockHookResult = {
  data: ExploreUpcomingShowsResponse | undefined
  isLoading: boolean
  error: Error | null
}

const mockUseExploreUpcomingShows = vi.fn<() => MockHookResult>(() => ({
  data: undefined,
  isLoading: false,
  error: null,
}))

vi.mock('../hooks', () => ({
  useExploreUpcomingShows: () => mockUseExploreUpcomingShows(),
}))

const sampleResponse: ExploreUpcomingShowsResponse = {
  shows: [
    {
      id: 1,
      slug: 'show-one',
      title: 'Show One',
      event_date: '2026-06-15T03:00:00Z',
      headliner_name: 'Headliner A',
      venue_name: 'The Trunk Space',
      venue_city: 'Phoenix',
      venue_state: 'AZ',
    },
    {
      id: 2,
      slug: 'show-two',
      title: 'Show Two',
      event_date: '2026-06-16T03:00:00Z',
      headliner_name: 'Headliner B',
      venue_name: 'Crescent Ballroom',
      venue_city: 'Phoenix',
      venue_state: 'AZ',
    },
  ],
  total: 2,
  limit: 5,
  offset: 0,
}

describe('UpcomingShowsList', () => {
  beforeEach(() => {
    mockUseExploreUpcomingShows.mockReset()
  })

  it('renders a loading spinner while fetching', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    const { container } = render(<UpcomingShowsList />)
    expect(container.querySelector('.animate-spin')).toBeTruthy()
  })

  it('renders an error message when the hook fails', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    render(<UpcomingShowsList />)
    expect(screen.getByText(/unable to load shows/i)).toBeInTheDocument()
  })

  it('renders the empty state when no shows come back', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: { shows: [], total: 0, limit: 5, offset: 0 },
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)
    expect(screen.getByText(/no upcoming shows/i)).toBeInTheDocument()
  })

  it('renders one row per show with a link to the show detail page', () => {
    mockUseExploreUpcomingShows.mockReturnValue({
      data: sampleResponse,
      isLoading: false,
      error: null,
    })
    render(<UpcomingShowsList />)

    const linkOne = screen.getByRole('link', { name: 'Show One' })
    expect(linkOne).toHaveAttribute('href', '/shows/show-one')
    expect(linkOne).toHaveTextContent('Headliner A')

    const linkTwo = screen.getByRole('link', { name: 'Show Two' })
    expect(linkTwo).toHaveAttribute('href', '/shows/show-two')
    expect(linkTwo).toHaveTextContent('Headliner B')
  })
})
