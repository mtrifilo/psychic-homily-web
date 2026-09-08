import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { getCategoryChipClasses } from '@/features/tags/types'
import type { SceneCrewSummary, SceneDetail } from '../types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseSceneCrews = vi.fn()
vi.mock('../hooks', () => ({
  useSceneCrews: (slug: unknown) => mockUseSceneCrews(slug),
}))

import { SceneCrews } from './SceneCrews'

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
    stats: {
      venue_count: 12,
      artist_count: 17,
      upcoming_show_count: 328,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 0,
      shows_prev_month: 0,
      shows_trend: 0,
      new_artists_30d: 0,
      active_venues_this_month: 0,
      shows_by_month: [],
    },
    venues: [],
    ...overrides,
  }
}

/** The endpoint's own order: most of the scene's shows first, then name. */
const CREWS: SceneCrewSummary[] = [
  { slug: 'relax-attack-jazz-series', name: 'Relax Attack Jazz Series', show_count: 3 },
  { slug: 'pleiades-series', name: 'Pleiades Series', show_count: 1 },
]

function mockCrews(crews: SceneCrewSummary[] | undefined) {
  mockUseSceneCrews.mockReturnValue({
    data: crews === undefined ? undefined : { crews },
  })
}

beforeEach(() => {
  mockUseSceneCrews.mockReset()
})

describe('SceneCrews', () => {
  it('keys the hook by slug', () => {
    mockCrews(CREWS)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(mockUseSceneCrews).toHaveBeenCalledWith('phoenix-az')
  })

  it('links each chip to the crew tag page', () => {
    mockCrews(CREWS)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(
      screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    ).toHaveAttribute('href', '/tags/relax-attack-jazz-series')
    expect(screen.getByRole('link', { name: 'Pleiades Series' })).toHaveAttribute(
      'href',
      '/tags/pleiades-series'
    )
  })

  // The backend ranks by the scene's show count and the row prints that order.
  // Re-sorting here would publish a different ranking than the one the payload
  // documents.
  it('renders the chips in the order the endpoint returned them', () => {
    mockCrews(CREWS)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    const names = screen
      .getAllByRole('listitem')
      .map(item => item.textContent)
    expect(names).toEqual(['Relax Attack Jazz Series', 'Pleiades Series'])
  })

  // No cap: the row wraps rather than truncating, so a crowded scene names
  // every crew booking in it.
  it('caps nothing', () => {
    const many = Array.from({ length: 14 }, (_, i) => ({
      slug: `crew-${i}`,
      name: `Crew ${i}`,
      show_count: 14 - i,
    }))
    mockCrews(many)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(screen.getAllByRole('listitem')).toHaveLength(14)
  })

  it('wears the crew category chip treatment', () => {
    mockCrews(CREWS)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    const chip = screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    for (const cls of getCategoryChipClasses('crew').split(' ')) {
      expect(chip).toHaveClass(cls)
    }
  })

  // The show counts rank the row; they are not the reader's business.
  it('does not print the ranking counts', () => {
    mockCrews(CREWS)
    const { container } = renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(container.textContent).not.toMatch(/\d/)
  })

  it('hides completely when the scene has no crew tags', () => {
    mockCrews([])
    const { container } = renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(container).toBeEmptyDOMElement()
  })

  // Loading and error both arrive as absent data, and neither may flash a row
  // the payload may not warrant.
  it('hides while there is no payload', () => {
    mockCrews(undefined)
    const { container } = renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(container).toBeEmptyDOMElement()
  })

  // A slug that resolves to the tag INDEX rather than to the crew: the chip is
  // still named, and it is not a link.
  it('names a crew it cannot link, without linking it', () => {
    mockCrews([{ slug: '', name: 'Unslugged Crew', show_count: 2 }])
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    const item = screen.getByRole('listitem')
    expect(item).toHaveTextContent('Unslugged Crew')
    expect(within(item).queryByRole('link')).not.toBeInTheDocument()
  })

  it('names the row after the scene the page is on', () => {
    mockCrews(CREWS)
    renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(
      screen.getByRole('list', { name: 'Crews booking in Phoenix' })
    ).toBeInTheDocument()
  })
})
