import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PrimaryNav } from './PrimaryNav'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

const mockAuth = vi.fn<() => { isAuthenticated: boolean }>(() => ({
  isAuthenticated: false,
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuth(),
}))

describe('PrimaryNav', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
    mockAuth.mockReturnValue({ isAuthenticated: false })
  })

  it('renders the explicit primary destinations with correct hrefs', () => {
    render(<PrimaryNav />)
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/')
    expect(screen.getByRole('link', { name: 'Explore' })).toHaveAttribute('href', '/explore')
    expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('href', '/shows')
    expect(screen.getByRole('link', { name: 'Artists' })).toHaveAttribute('href', '/artists')
  })

  it('marks Home active on the home route only', () => {
    mockPathname = '/'
    render(<PrimaryNav />)
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Shows' })).not.toHaveAttribute('aria-current')
  })

  it('marks Shows active on a shows sub-route', () => {
    mockPathname = '/shows/some-show'
    render(<PrimaryNav />)
    expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Home' })).not.toHaveAttribute('aria-current')
  })

  it('renders the Radio, Browse, and Contribute menu triggers', () => {
    render(<PrimaryNav />)
    expect(screen.getByRole('button', { name: 'Radio' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Browse the catalog' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Contribute' })).toBeInTheDocument()
  })

  it('opens Browse and reaches the catalog (sidebar destinations stay reachable)', async () => {
    const user = userEvent.setup()
    render(<PrimaryNav />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))
    expect(await screen.findByRole('menuitem', { name: 'Venues' })).toHaveAttribute('href', '/venues')
    expect(screen.getByRole('menuitem', { name: 'Labels' })).toHaveAttribute('href', '/labels')
    expect(screen.getByRole('menuitem', { name: 'Collections' })).toHaveAttribute('href', '/collections')
  })

  it('hides the auth-only Contribute item when signed out', async () => {
    const user = userEvent.setup()
    render(<PrimaryNav />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))
    expect(await screen.findByRole('menuitem', { name: 'Submit a Show' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'My Submissions' })).not.toBeInTheDocument()
  })

  it('shows the auth-only Contribute item when signed in', async () => {
    mockAuth.mockReturnValue({ isAuthenticated: true })
    const user = userEvent.setup()
    render(<PrimaryNav />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))
    expect(await screen.findByRole('menuitem', { name: 'My Submissions' })).toHaveAttribute('href', '/submissions')
  })
})
