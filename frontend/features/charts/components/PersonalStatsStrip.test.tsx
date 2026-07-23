import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

let isAuthenticated = true
let followedLooseEndsCount = 0

const personalStats = {
  saved_shows: 12,
  artists_followed: 34,
  venues_followed: 0,
  labels_followed: 0,
  scenes_followed: 0,
  festivals_followed: 0,
  top_venue: null as null | {
    venue_id: number
    name: string
    slug: string
    saved_show_count: number
  },
  first_activity_at: null as string | null,
  top_scenes: [] as Array<{
    metro: string
    name: string
    slug: string
    city: string
    state: string
    count: number
  }>,
  top_tags: [] as Array<{
    tag_id: number
    name: string
    slug: string
    category: string
    count: number
  }>,
  top_artists: [] as Array<{
    artist_id: number
    name: string
    slug: string
    count: number
  }>,
}

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated,
    isLoading: false,
    user: isAuthenticated ? { id: '42' } : null,
  }),
}))

vi.mock('../hooks', () => ({
  usePersonalChartsStats: (_userId: string | undefined, enabled: boolean) =>
    enabled
      ? {
          data: personalStats,
          isLoading: false,
          isError: false,
          isSuccess: true,
          isFetching: false,
        }
      : {
          data: undefined,
          isLoading: false,
          isError: false,
          isSuccess: false,
          isFetching: false,
        },
}))

vi.mock('@/features/contributions', () => ({
  FOLLOWED_LOOSE_ENDS_KEY: 'followed_artists_missing_links',
  useContributeOpportunities: () => ({
    data: {
      categories: [
        {
          key: 'followed_artists_missing_links',
          label: 'Artists you follow missing links',
          entity_type: 'artist',
          count: followedLooseEndsCount,
          description: '',
        },
      ],
      total_items: followedLooseEndsCount,
    },
    isLoading: false,
    isError: false,
  }),
}))

// Import after mocks are wired.
import { PersonalStatsStrip } from './PersonalStatsStrip'

describe('PersonalStatsStrip — Loose Ends deep link', () => {
  beforeEach(() => {
    isAuthenticated = true
    followedLooseEndsCount = 0
  })

  it('links to /contribute when the viewer follows artists with loose ends', () => {
    followedLooseEndsCount = 3

    render(<PersonalStatsStrip />)

    const link = screen.getByRole('link', {
      name: /loose ends in your follows/i,
    })
    expect(link).toHaveAttribute('href', '/contribute')
    expect(link).toHaveTextContent('3 loose ends in your follows')
  })

  it('uses the singular "loose end" for a count of one', () => {
    followedLooseEndsCount = 1

    render(<PersonalStatsStrip />)

    expect(
      screen.getByRole('link', { name: /1 loose end in your follows/i })
    ).toBeInTheDocument()
  })

  it('does not render the deep link when the followed loose-ends count is zero', () => {
    followedLooseEndsCount = 0

    render(<PersonalStatsStrip />)

    // Strip still renders (authed with stats), just without the deep link.
    expect(screen.getByLabelText('Your chart stats')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /loose ends/i })
    ).not.toBeInTheDocument()
  })

  it('renders nothing for anonymous viewers even with loose ends present', () => {
    isAuthenticated = false
    followedLooseEndsCount = 5

    const { container } = render(<PersonalStatsStrip />)

    expect(container).toBeEmptyDOMElement()
  })
})
