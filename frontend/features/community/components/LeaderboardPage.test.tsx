import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    COMMUNITY: {
      LEADERBOARD: '/community/leaderboard',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    community: {
      leaderboard: (dimension: string, period: string, limit?: number) =>
        ['community', 'leaderboard', dimension, period, limit],
    },
  },
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: { id: 1, username: 'currentuser' },
    isAuthenticated: true,
  }),
}))

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn() }),
  usePathname: () => '/community/leaderboard',
  useSearchParams: () => new URLSearchParams(),
}))

import { LeaderboardPage } from './LeaderboardPage'

const mockLeaderboardData = {
  entries: [
    {
      rank: 1,
      user_id: 2,
      username: 'alice',
      avatar_url: null,
      user_tier: 'trusted_contributor',
      count: 150,
    },
    {
      rank: 2,
      user_id: 1,
      username: 'currentuser',
      avatar_url: null,
      user_tier: 'contributor',
      count: 100,
    },
    {
      rank: 3,
      user_id: 3,
      username: 'bob',
      avatar_url: null,
      user_tier: 'new_user',
      count: 50,
    },
  ],
  dimension: 'overall',
  period: 'all_time',
}

describe('LeaderboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('renders leaderboard table with entries', async () => {
    mockApiRequest.mockResolvedValueOnce(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('alice')).toBeInTheDocument()
    })

    expect(screen.getByText('currentuser')).toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
    expect(screen.getByText('150')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('50')).toBeInTheDocument()
  })

  it('highlights current user row', async () => {
    mockApiRequest.mockResolvedValueOnce(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('currentuser')).toBeInTheDocument()
    })

    // Current user should have "(you)" indicator
    expect(screen.getByText('(you)')).toBeInTheDocument()
  })

  it('shows tier badges', async () => {
    mockApiRequest.mockResolvedValueOnce(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('Trusted')).toBeInTheDocument()
    })

    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('New')).toBeInTheDocument()
  })

  it('tab switching changes dimension', async () => {
    const user = userEvent.setup()
    mockApiRequest.mockResolvedValue(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('alice')).toBeInTheDocument()
    })

    // Click the "Shows" tab
    await user.click(screen.getByRole('tab', { name: 'Shows' }))

    // Should refetch with new dimension
    await waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalledWith(
        expect.stringContaining('dimension=shows'),
        expect.any(Object),
      )
    })
  })

  it('period filter works', async () => {
    const user = userEvent.setup()
    mockApiRequest.mockResolvedValue(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('alice')).toBeInTheDocument()
    })

    // Change period to "This Week"
    const select = screen.getByRole('combobox')
    await user.selectOptions(select, 'week')

    await waitFor(() => {
      expect(mockApiRequest).toHaveBeenCalledWith(
        expect.stringContaining('period=week'),
        expect.any(Object),
      )
    })
  })

  it('shows empty state when no entries', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entries: [],
      dimension: 'overall',
      period: 'all_time',
    })

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('No contributions yet')).toBeInTheDocument()
    })

    expect(screen.getByText(/Be the first/)).toBeInTheDocument()
  })

  it('shows loading skeleton', () => {
    // Never resolve the request to keep loading state
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    renderWithProviders(<LeaderboardPage />)

    // Skeleton items should be visible (animated pulse divs)
    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('links to user profiles', async () => {
    mockApiRequest.mockResolvedValueOnce(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    await waitFor(() => {
      expect(screen.getByText('alice')).toBeInTheDocument()
    })

    const aliceLink = screen.getByText('alice').closest('a')
    expect(aliceLink).toHaveAttribute('href', '/users/alice')

    const bobLink = screen.getByText('bob').closest('a')
    expect(bobLink).toHaveAttribute('href', '/users/bob')
  })

  it('renders all dimension tabs', async () => {
    mockApiRequest.mockResolvedValueOnce(mockLeaderboardData)

    renderWithProviders(<LeaderboardPage />)

    expect(screen.getByRole('tab', { name: 'Overall' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Shows' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Venues' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Tags' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Edits' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Requests' })).toBeInTheDocument()
  })
})
