import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// The sections around the list are stood in: this suite is about where the
// list slot lands, not what the other sections render.
vi.mock('./HomeHero', () => ({ HomeHero: () => null }))
vi.mock('./CommunityPulseBand', () => ({ CommunityPulseBand: () => null }))
vi.mock('./HomeSceneGraph', () => ({
  HomeSceneGraph: ({ narrowForm }: { narrowForm: string }) => (
    <div data-testid="scene-graph" data-narrow-form={narrowForm} />
  ),
}))
vi.mock('./LatestRadioShows', () => ({
  LatestRadioShows: ({ variant }: { variant: string }) => (
    <div data-testid="radio" data-variant={variant} />
  ),
}))
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

describe('AnonymousHome shared sections', () => {
  // The signed-in home's rows and link row must not leak onto this page.
  it('keeps the radio cards and the map teaser', () => {
    render(<AnonymousHome />)

    expect(screen.getByTestId('radio')).toHaveAttribute('data-variant', 'cards')
    expect(screen.getByTestId('scene-graph')).toHaveAttribute(
      'data-narrow-form',
      'teaser'
    )
  })
})
