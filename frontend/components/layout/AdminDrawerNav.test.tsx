import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import AdminDrawerNav from './AdminDrawerNav'

const mockPathname = vi.fn(() => '/admin')
let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname(),
  useSearchParams: () => mockSearchParams,
}))

const mockNavCounts = vi.fn(() => ({
  moderation: 0,
  pendingShows: 0,
  unverifiedVenues: 0,
  reports: 0,
}))
vi.mock('@/lib/hooks/admin/useAdminNavCounts', () => ({
  useAdminNavCounts: () => mockNavCounts(),
}))

const ACTIVE = 'bg-accent text-accent-foreground'

describe('AdminDrawerNav', () => {
  const onNavigate = vi.fn()

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
    render(<AdminDrawerNav onNavigate={onNavigate} />)
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
    render(<AdminDrawerNav onNavigate={onNavigate} />)
    expect(screen.getByText('Moderation').closest('a')!.className).toContain(ACTIVE)
    expect(screen.getByText('Releases').closest('a')!.className).not.toContain(ACTIVE)
  })

  it('renders queue badges and omits zero counts', () => {
    mockNavCounts.mockReturnValue({
      moderation: 5,
      pendingShows: 2,
      unverifiedVenues: 0,
      reports: 3,
    })
    render(<AdminDrawerNav onNavigate={onNavigate} />)
    expect(within(screen.getByText('Moderation').closest('a')!).getByText('5')).toBeInTheDocument()
    expect(within(screen.getByText('Reports').closest('a')!).getByText('3')).toBeInTheDocument()
    expect(
      within(screen.getByText('Unverified Venues').closest('a')!).queryByText('0')
    ).not.toBeInTheDocument()
  })

  it('closes the drawer when an admin section is clicked', async () => {
    const user = userEvent.setup()
    render(<AdminDrawerNav onNavigate={onNavigate} />)
    await user.click(screen.getByText('Moderation'))
    expect(onNavigate).toHaveBeenCalledTimes(1)
  })

  it('Back to site points at / and closes the drawer', async () => {
    const user = userEvent.setup()
    render(<AdminDrawerNav onNavigate={onNavigate} />)
    const back = screen.getByText('Back to site').closest('a')!
    expect(back).toHaveAttribute('href', '/')
    await user.click(back)
    expect(onNavigate).toHaveBeenCalledTimes(1)
  })
})
