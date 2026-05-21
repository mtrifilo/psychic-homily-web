import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import UnverifiedVenuesPage from './page'

// The page renders its header + content inline from useUnverifiedVenues(). The
// card/dialog subcomponents live in the same file, so we drive the empty and
// populated branches via the hook mock rather than stubbing children.

let mockVenues: {
  data: { venues: { id: number; name: string; city: string; state: string; show_count: number; created_at: string }[]; total: number } | undefined
  isLoading: boolean
  error: unknown
}

vi.mock('@/lib/hooks/admin/useAdminVenues', () => ({
  useUnverifiedVenues: () => mockVenues,
  useVerifyVenue: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: { is_admin: true } }),
}))

describe('UnverifiedVenuesPage (app/admin/unverified-venues)', () => {
  beforeEach(() => {
    mockVenues = { data: undefined, isLoading: false, error: null }
  })

  it('renders the page heading without throwing', () => {
    render(<UnverifiedVenuesPage />)

    expect(
      screen.getByRole('heading', { name: 'Unverified Venues' })
    ).toBeInTheDocument()
  })

  it('renders the all-verified empty state', () => {
    mockVenues = { data: { venues: [], total: 0 }, isLoading: false, error: null }

    render(<UnverifiedVenuesPage />)

    expect(
      screen.getByRole('heading', { name: 'All Venues Verified' })
    ).toBeInTheDocument()
  })

  it('renders an unverified venue card and the awaiting-review count', () => {
    mockVenues = {
      data: {
        venues: [
          {
            id: 1,
            name: 'The Venue',
            city: 'Phoenix',
            state: 'AZ',
            show_count: 3,
            created_at: '2026-04-01T00:00:00Z',
          },
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    }

    render(<UnverifiedVenuesPage />)

    expect(
      screen.getByRole('heading', { name: 'The Venue' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('1 unverified venue awaiting review')
    ).toBeInTheDocument()
  })
})
