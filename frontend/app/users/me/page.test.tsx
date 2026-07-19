import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, screen } from '@/test/utils'
import SelfProfilePage from './page'

const mockReplace = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseOwnContributorProfile = vi.fn()
const mockUseMyCollections = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

vi.mock('@/features/auth', () => ({
  useOwnContributorProfile: () => mockUseOwnContributorProfile(),
}))

vi.mock('@/features/collections', () => ({
  useMyCollections: () => mockUseMyCollections(),
}))

vi.mock('@/features/profile', () => ({
  GetStartedChecklist: () => <div data-testid="get-started-checklist" />,
  ProfileStatsSidebar: () => <div data-testid="profile-stats-sidebar" />,
  UserTierBadge: ({ tier }: { tier: string }) => (
    <span data-testid="user-tier-badge">{tier}</span>
  ),
}))

const CLAIM_USER = {
  id: '1',
  email: 'alice@example.com',
  display_name: 'Alice Anderson',
  // No username — claim state only renders when username is absent.
}

function setClaimAuth(userOverrides: Record<string, unknown> = {}) {
  mockUseAuthContext.mockReturnValue({
    isAuthenticated: true,
    isLoading: false,
    user: { ...CLAIM_USER, ...userOverrides },
  })
}

describe('SelfProfilePage claim state (PSY-1488)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setClaimAuth()
    mockUseOwnContributorProfile.mockReturnValue({
      data: undefined,
      isLoading: false,
    })
    mockUseMyCollections.mockReturnValue({
      data: { total: 0 },
      isLoading: false,
    })
  })

  it('renders OAuth avatar image when user.avatar_url is present', () => {
    setClaimAuth({ avatar_url: 'https://example.com/oauth-avatar.jpg' })

    renderWithProviders(<SelfProfilePage />)

    const img = screen.getByAltText("Alice Anderson's avatar")
    expect(img).toHaveAttribute('src', 'https://example.com/oauth-avatar.jpg')
    expect(img).toHaveClass('h-16', 'w-16', 'rounded-full', 'object-cover')
    expect(screen.queryByText('A')).toBeNull()
  })

  it('falls back to contributor profile avatar_url when user has none', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: {
        avatar_url: 'https://example.com/profile-avatar.jpg',
        display_name: 'Alice Anderson',
      },
      isLoading: false,
    })

    renderWithProviders(<SelfProfilePage />)

    const img = screen.getByAltText("Alice Anderson's avatar")
    expect(img).toHaveAttribute('src', 'https://example.com/profile-avatar.jpg')
  })

  it('falls back to initials when no avatar_url is present', () => {
    renderWithProviders(<SelfProfilePage />)

    expect(screen.getByText('A')).toBeInTheDocument()
    expect(screen.queryByAltText(/avatar/i)).toBeNull()
  })

  it('keeps PSY-1485 claim CTAs pointing at /profile#username and /profile#bio', () => {
    renderWithProviders(<SelfProfilePage />)

    expect(screen.getByRole('link', { name: /set username/i })).toHaveAttribute(
      'href',
      '/profile#username'
    )
    expect(screen.getByRole('link', { name: /add bio/i })).toHaveAttribute(
      'href',
      '/profile#bio'
    )
  })
})
