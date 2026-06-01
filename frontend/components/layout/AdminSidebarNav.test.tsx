import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { TooltipProvider } from '@/components/ui/tooltip'
import AdminSidebarNav from './AdminSidebarNav'

// AdminSidebarNav derives the active section from usePathname + ?tab=.
const mockPathname = vi.fn(() => '/admin')
let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
  useSearchParams: () => mockSearchParams,
}))

// Stub the counts hook so we don't need a QueryClientProvider; the aggregation +
// gating contract are covered by useAdminNavCounts.test.ts.
const mockNavCounts = vi.fn(() => ({
  moderation: 0,
  pendingShows: 0,
  unverifiedVenues: 0,
  reports: 0,
}))
vi.mock('@/lib/hooks/admin/useAdminNavCounts', () => ({
  useAdminNavCounts: () => mockNavCounts(),
}))

const ACTIVE_TOKEN = 'bg-sidebar-accent text-sidebar-accent-foreground'

// Tooltips (collapsed mode) require a TooltipProvider — Sidebar's aside supplies
// one in the app; provide it here.
function renderNav(collapsed = false) {
  return render(
    <TooltipProvider>
      <AdminSidebarNav collapsed={collapsed} />
    </TooltipProvider>
  )
}

describe('AdminSidebarNav', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname.mockReturnValue('/admin')
    mockSearchParams = new URLSearchParams()
    mockNavCounts.mockReturnValue({
      moderation: 0,
      pendingShows: 0,
      unverifiedVenues: 0,
      reports: 0,
    })
  })

  it('renders the 6 admin group headers + Back to site', () => {
    renderNav()
    for (const h of [
      'Overview',
      'Moderation & Queues',
      'Catalog',
      'Curation & Taxonomy',
      'Tools',
      'Insights & System',
    ]) {
      expect(screen.getByText(h)).toBeInTheDocument()
    }
    expect(screen.getByText('Back to site').closest('a')).toHaveAttribute('href', '/')
  })

  it('marks the section matching ?tab= as active', () => {
    mockSearchParams = new URLSearchParams('tab=moderation')
    renderNav()
    expect(screen.getByText('Moderation').closest('a')!.className).toContain(ACTIVE_TOKEN)
    expect(screen.getByText('Releases').closest('a')!.className).not.toContain(ACTIVE_TOKEN)
  })

  it('treats bare /admin (no tab) as Dashboard active', () => {
    renderNav()
    expect(screen.getByText('Dashboard').closest('a')!.className).toContain(ACTIVE_TOKEN)
  })

  it('renders queue count badges and omits zero counts', () => {
    mockNavCounts.mockReturnValue({
      moderation: 5,
      pendingShows: 2,
      unverifiedVenues: 0,
      reports: 3,
    })
    renderNav()
    expect(within(screen.getByText('Moderation').closest('a')!).getByText('5')).toBeInTheDocument()
    expect(within(screen.getByText('Reports').closest('a')!).getByText('3')).toBeInTheDocument()
    expect(
      within(screen.getByText('Unverified Venues').closest('a')!).queryByText('0')
    ).not.toBeInTheDocument()
  })

  it('collapsed: a queued section shows a corner dot (not the number)', () => {
    mockNavCounts.mockReturnValue({
      moderation: 5,
      pendingShows: 0,
      unverifiedVenues: 0,
      reports: 0,
    })
    renderNav(true)
    const links = Array.from(document.querySelectorAll('a'))
    const moderation = links.find(a => a.getAttribute('href') === '/admin?tab=moderation')!
    expect(moderation).toBeTruthy()
    expect(moderation.querySelector('.bg-purple-500')).toBeTruthy()
    expect(moderation.textContent).not.toContain('5')
  })
})
