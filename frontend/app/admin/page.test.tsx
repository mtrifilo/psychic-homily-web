import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import AdminPage from './page'

// The admin index page (app/admin/page.tsx) is the tabbed shell. It reads the
// active tab from the URL, gates on the authenticated admin, and lazy-loads
// each tab's panel via next/dynamic. We mount it with an admin user + the
// badge-count hooks mocked and assert the console shell + tab bar render, and
// that the default (dashboard) panel resolves with its stats panels.
//
// The tab panels are dynamically imported via next/dynamic. We replace it with
// a component that resolves the loader in an effect and renders the result.
// (React.lazy would suspend the whole AdminPageContent tree — including the
// eager tab bar — under the page's single top-level <Suspense>, so the tab bar
// would not be queryable synchronously. An effect-driven swap keeps the shell
// eager and resolves each panel asynchronously, matching next/dynamic's shape.)
vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: (loader: () => Promise<unknown>) =>
      function DynamicMock(props: Record<string, unknown>) {
        const [Comp, setComp] =
          React.useState<React.ComponentType<Record<string, unknown>> | null>(null)
        React.useEffect(() => {
          let active = true
          Promise.resolve(loader()).then((mod) => {
            if (!active) return
            const resolved =
              typeof mod === 'function'
                ? mod
                : (mod as { default?: React.ComponentType }).default ?? (() => null)
            setComp(() => resolved as React.ComponentType<Record<string, unknown>>)
          })
          return () => {
            active = false
          }
        }, [])
        return Comp ? <Comp {...props} /> : null
      },
  }
})

// The default (dashboard) tab lazy-imports ./dashboard/page → <AdminDashboard>,
// which renders its stat panels from useAdminStats / useAdminActivity. Mock
// those two hooks so the real dashboard skeleton (Needs Attention / Platform /
// Recent Activity panels) renders without standing up the network layer.
const mockStats = {
  pending_shows: 0,
  pending_venue_edits: 0,
  pending_reports: 0,
  unverified_venues: 0,
  total_shows: 42,
  total_venues: 10,
  total_artists: 30,
  total_users: 12,
  shows_submitted_last_7_days: 3,
  users_registered_last_7_days: 2,
  total_shows_trend: 1,
  total_venues_trend: 0,
  total_artists_trend: 2,
  total_users_trend: 1,
}

vi.mock('@/lib/hooks/admin/useAdminStats', () => ({
  useAdminStats: () => ({ data: mockStats, isLoading: false, error: null }),
  useAdminActivity: () => ({ data: { events: [] }, isLoading: false, error: null }),
}))

const mockReplace = vi.fn()
let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: { is_admin: true },
    isAuthenticated: true,
    isLoading: false,
  }),
}))

// Badge-count hooks — the shell sums these into the moderation/reports badges.
const emptyQuery = { data: undefined }

vi.mock('@/lib/hooks/admin/useAdminVenues', () => ({
  useUnverifiedVenues: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminReports', () => ({
  usePendingReports: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminArtistReports', () => ({
  usePendingArtistReports: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  usePendingShows: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminPendingEdits', () => ({
  useAdminPendingEdits: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminEntityReports', () => ({
  useAdminEntityReports: () => emptyQuery,
}))
vi.mock('@/lib/hooks/admin/useAdminComments', () => ({
  useAdminPendingComments: () => emptyQuery,
}))

describe('AdminPage (app/admin shell)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams = new URLSearchParams()
    // jsdom doesn't implement Element scroll methods. The page's
    // ScrollableTabBar calls scrollTo/scrollBy from a layout effect to keep the
    // active tab in view; stub them so those effects don't throw.
    Element.prototype.scrollTo = vi.fn()
    Element.prototype.scrollBy = vi.fn()
  })

  it('renders the Admin Console header for an authenticated admin', () => {
    renderWithProviders(<AdminPage />)

    expect(
      screen.getByRole('heading', { name: 'Admin Console' })
    ).toBeInTheDocument()
  })

  it('renders the full tab bar', () => {
    renderWithProviders(<AdminPage />)

    for (const tab of [
      'Dashboard',
      'Moderation',
      'Pending Shows',
      'Unverified Venues',
      'Reports',
      'Releases',
      'Labels',
      'Festivals',
      'Tags',
      'Data Quality',
      'Analytics',
      'Radio',
      'Users',
      'Audit Log',
    ]) {
      expect(screen.getByRole('tab', { name: new RegExp(tab) })).toBeInTheDocument()
    }
  })

  it('renders the dashboard skeleton panels on the default tab', async () => {
    renderWithProviders(<AdminPage />)

    // The dashboard panel is lazy-loaded; wait for the dynamic import to resolve.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Needs Attention' })
      ).toBeInTheDocument()
    )
    expect(
      screen.getByRole('heading', { name: 'Platform' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Recent Activity' })
    ).toBeInTheDocument()
  })

  it('does not redirect an authenticated admin away from the console', () => {
    renderWithProviders(<AdminPage />)

    expect(mockReplace).not.toHaveBeenCalled()
  })
})
