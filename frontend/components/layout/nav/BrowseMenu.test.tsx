import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowseMenu } from './BrowseMenu'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

describe('BrowseMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
  })

  it('opens on click and renders the three scent-rich column headers', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))
    // Column headers carry no role; assert their text is present once open.
    expect(await screen.findByText('Catalog')).toBeInTheDocument()
    expect(screen.getByText('Curation')).toBeInTheDocument()
    expect(screen.getByText('Scenes')).toBeInTheDocument()
  })

  it('every catalog + curation item is a real link to its route', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))

    const cases: Array<[string, string]> = [
      ['Artists', '/artists'],
      ['Venues', '/venues'],
      ['Releases', '/releases'],
      ['Labels', '/labels'],
      ['Festivals', '/festivals'],
      ['Collections', '/collections'],
      ['Charts', '/charts'],
      ['Tags', '/tags'],
      ['Leaderboard', '/community/leaderboard'],
    ]
    for (const [name, href] of cases) {
      expect(await screen.findByRole('menuitem', { name })).toHaveAttribute('href', href)
    }
  })

  it('renders the curated scene shortcuts with the derived <city>-<state> slug shape', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))

    expect(await screen.findByRole('menuitem', { name: 'Phoenix' })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az'
    )
    expect(screen.getByRole('menuitem', { name: 'Tucson' })).toHaveAttribute(
      'href',
      '/scenes/tucson-az'
    )
    expect(screen.getByRole('menuitem', { name: 'Los Angeles' })).toHaveAttribute(
      'href',
      '/scenes/los-angeles-ca'
    )
    expect(screen.getByRole('menuitem', { name: 'All scenes' })).toHaveAttribute('href', '/scenes')
  })

  it('opens via keyboard (Enter on the focused trigger) — APG menu pattern', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    const trigger = screen.getByRole('button', { name: 'Browse the catalog' })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('menuitem', { name: 'Artists' })).toBeInTheDocument()
  })

  it('Escape closes the menu and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    const trigger = screen.getByRole('button', { name: 'Browse the catalog' })
    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: 'Artists' })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('menuitem', { name: 'Artists' })).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('marks the trigger active when on a destination route', () => {
    mockPathname = '/venues'
    render(<BrowseMenu />)
    // navItemClassName applies the active (foreground) treatment; assert the
    // active class is present so the trigger reads as selected.
    expect(screen.getByRole('button', { name: 'Browse the catalog' }).className).toContain(
      'text-foreground'
    )
  })
})
