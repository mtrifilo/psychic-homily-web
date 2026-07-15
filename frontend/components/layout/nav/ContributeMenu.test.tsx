import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ContributeMenu } from './ContributeMenu'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

let mockIsAuthenticated = false
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: mockIsAuthenticated }),
}))

describe('ContributeMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
    mockIsAuthenticated = false
  })

  it('opens on click and renders both column headers', async () => {
    const user = userEvent.setup()
    render(<ContributeMenu />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))
    expect(await screen.findByText('Participate')).toBeInTheDocument()
    expect(screen.getByText('Editorial')).toBeInTheDocument()
  })

  it('renders the Participate + Editorial items as real links to their routes', async () => {
    const user = userEvent.setup()
    render(<ContributeMenu />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))

    const cases: Array<[string, string]> = [
      ['+ Submit a show', '/shows/submit'],
      ['Requests', '/requests'],
      ['Leaderboard', '/community/leaderboard'],
      ['Contribute hub →', '/contribute'],
      ['Blog', '/blog'],
      ['DJ Sets', '/dj-sets'],
      ['Substack ↗', 'https://psychichomily.substack.com/'],
    ]
    for (const [name, href] of cases) {
      expect(await screen.findByRole('menuitem', { name })).toHaveAttribute('href', href)
    }
  })

  it('hides both auth-only submission destinations when logged out', async () => {
    const user = userEvent.setup()
    render(<ContributeMenu />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))
    expect(await screen.findByText('Participate')).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Show Submissions' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'My Submissions' })).not.toBeInTheDocument()
  })

  it('distinguishes show submissions from pending entity edits when authenticated', async () => {
    mockIsAuthenticated = true
    const user = userEvent.setup()
    render(<ContributeMenu />)
    await user.click(screen.getByRole('button', { name: 'Contribute' }))
    expect(await screen.findByRole('menuitem', { name: 'Show Submissions' })).toHaveAttribute(
      'href',
      '/contribute/submissions'
    )
    expect(await screen.findByRole('menuitem', { name: 'My Submissions' })).toHaveAttribute(
      'href',
      '/submissions'
    )
  })

  it('opens via keyboard (Enter on the focused trigger) — APG menu pattern', async () => {
    const user = userEvent.setup()
    render(<ContributeMenu />)
    const trigger = screen.getByRole('button', { name: 'Contribute' })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('menuitem', { name: 'Requests' })).toBeInTheDocument()
  })

  it('Escape closes the menu and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<ContributeMenu />)
    const trigger = screen.getByRole('button', { name: 'Contribute' })
    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: 'Requests' })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('menuitem', { name: 'Requests' })).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('marks the trigger active when on a destination route', () => {
    mockPathname = '/requests'
    render(<ContributeMenu />)
    expect(screen.getByRole('button', { name: 'Contribute' }).className).toContain('text-foreground')
  })
})
