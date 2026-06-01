import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import AdminPage from './page'

// The admin index page (app/admin/page.tsx) is the tabbed shell. It derives the
// active section from the URL ?tab= param, gates on the authenticated admin, and
// lazy-loads each section's panel via next/dynamic. Section navigation now lives
// in the context-aware Sidebar / mobile drawer (PSY-933), NOT an in-page tab bar.
// We mount it with an admin user and assert the console shell renders and the
// default (dashboard) panel resolves with its stats panels.
//
// The tab panels are dynamically imported via next/dynamic. We replace it with
// a component that resolves the loader in an effect and renders the result.
// (React.lazy would suspend the whole AdminPageContent tree — including the
// eager shell header — under the page's single top-level <Suspense>, so the
// header would not be queryable synchronously. An effect-driven swap keeps the
// shell eager and resolves each panel asynchronously, matching next/dynamic's shape.)
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
  useAdminStats: () => ({ data: mockStats, isLoading: false, error: null as Error | null }),
  useAdminActivity: () => ({ data: { events: [] as unknown[] }, isLoading: false, error: null as Error | null }),
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

// The badge-count hooks moved out of this page to the Sidebar (PSY-933), so the
// shell no longer imports them — no mocks needed here.

describe('AdminPage (app/admin shell)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams = new URLSearchParams()
  })

  it('renders the Admin Console header for an authenticated admin', () => {
    renderWithProviders(<AdminPage />)

    expect(
      screen.getByRole('heading', { name: 'Admin Console' })
    ).toBeInTheDocument()
  })

  it('no longer renders an in-page tab bar (nav moved to the context-aware Sidebar, PSY-933)', () => {
    renderWithProviders(<AdminPage />)

    // The 18-tab ScrollableTabBar was retired; section navigation now lives in
    // the Sidebar / mobile drawer. The page is a value-controlled content shell.
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(screen.queryAllByRole('tab')).toHaveLength(0)
  })

  it('selects the section panel matching the ?tab= param (deep-link contract)', () => {
    // The nav-removal relies on activeTab deriving from ?tab=. Radix keeps every
    // TabsContent mounted but marks only the active one data-state="active" (the
    // rest are hidden) — so a deep-link to ?tab=moderation makes moderation the
    // active panel and dashboard inactive, proving the URL drives the section.
    mockSearchParams = new URLSearchParams('tab=moderation')
    renderWithProviders(<AdminPage />)

    expect(screen.getByTestId('admin-tab-moderation')).toHaveAttribute('data-state', 'active')
    expect(screen.getByTestId('admin-tab-dashboard')).toHaveAttribute('data-state', 'inactive')
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
