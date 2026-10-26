import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// The sections around the list are stood in: this suite is about where the
// list slot lands, not what the other sections render.
vi.mock('./HomeHero', () => ({ HomeHero: () => null }))
vi.mock('./CommunityPulseBand', () => ({ CommunityPulseBand: () => null }))
vi.mock('./HomeSceneGraph', () => ({ HomeSceneGraph: () => null }))
vi.mock('./LatestRadioShows', () => ({ LatestRadioShows: () => null }))
vi.mock('@/features/shows/components/HomeShowList', () => ({
  HomeShowList: () => <div data-testid="client-home-show-list" />,
}))

import { AnonymousHome } from './AnonymousHome'

describe('AnonymousHome upcoming-shows slot', () => {
  // The server variant hands its seeded list in through this slot; a list
  // rendered here in its place would silently drop the server-rendered rows.
  it('renders the list it is handed under the "Upcoming shows" heading', () => {
    render(<AnonymousHome upcomingShows={<div data-testid="seeded-list" />} />)

    const section = screen
      .getByRole('heading', { name: 'Upcoming shows' })
      .closest('section')
    expect(section).toContainElement(screen.getByTestId('seeded-list'))
    expect(screen.queryByTestId('client-home-show-list')).not.toBeInTheDocument()
  })

  it('renders the client list when no list is handed in', () => {
    render(<AnonymousHome />)

    expect(screen.getByTestId('client-home-show-list')).toBeInTheDocument()
  })
})
