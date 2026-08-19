import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneDetail, SceneNewArtistRow } from '../types'

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

const mockUseSceneNewArtists = vi.fn()
vi.mock('../hooks', () => ({
  useSceneNewArtists: (options: unknown) => mockUseSceneNewArtists(options),
}))

import { SceneNewBands } from './SceneNewBands'

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
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

function band(overrides: Partial<SceneNewArtistRow> = {}): SceneNewArtistRow {
  return {
    id: 1,
    name: 'Saguaro Teeth',
    slug: 'saguaro-teeth',
    first_listed_at: '2026-07-14T12:00:00Z',
    show: {
      id: 10,
      event_date: '2026-08-22',
      starts_at: '2026-08-23T02:00:00Z',
      is_upcoming: true,
      venue_name: 'Nile Theater',
    },
    ...overrides,
  }
}

/** The only thing that varies between these tests is the band list itself. */
function givenNewBands(artists: SceneNewArtistRow[]) {
  mockUseSceneNewArtists.mockReturnValue({ data: { artists } })
}

describe('SceneNewBands', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('names each band with the fact the ordering selected on', () => {
    givenNewBands([band()])
    renderWithProviders(<SceneNewBands scene={buildScene()} />)

    expect(
      screen.getByRole('heading', { name: /New \/ first listed in Phoenix/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Saguaro Teeth' })).toHaveAttribute(
      'href',
      '/artists/saguaro-teeth'
    )
    expect(
      screen.getByText('first listed Jul 14 · next show Aug 22, Nile Theater')
    ).toBeInTheDocument()
  })

  it('states the honest absence for a band with no show on file', () => {
    mockUseSceneNewArtists.mockReturnValue({
      data: {
        artists: [band({ id: 2, name: 'Nite Sweats', slug: 'nite-sweats', show: undefined })],
      },
    })
    renderWithProviders(<SceneNewBands scene={buildScene()} />)
    expect(
      screen.getByText('first listed Jul 14 · no show listed yet')
    ).toBeInTheDocument()
  })

  // Entity slugs are nullable and can generate as "", and `/artists/` resolves
  // to the artists INDEX rather than 404ing.
  it('names a slugless band without linking it', () => {
    givenNewBands([band({ slug: '' })])
    renderWithProviders(<SceneNewBands scene={buildScene()} />)
    expect(screen.getByText('Saguaro Teeth')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Saguaro Teeth' })
    ).not.toBeInTheDocument()
  })

  // The endpoint's own default owns the cap (PSY-1844), so the module must not
  // send a limit of its own — a third copy of the number the backend decides.
  it('lets the endpoint default own the cap', () => {
    givenNewBands([band()])
    renderWithProviders(<SceneNewBands scene={buildScene()} />)
    expect(mockUseSceneNewArtists).toHaveBeenCalledWith({ slug: 'phoenix-az' })
  })

  // Decision 4, the whole point of the module: the pulse's bare integer is
  // gone, and a scene with nothing new has no section rather than a zero.
  describe('the zero state', () => {
    it('renders nothing at all when the scene has no bands to name', () => {
      mockUseSceneNewArtists.mockReturnValue({ data: { artists: [] } })
      const { container } = renderWithProviders(<SceneNewBands scene={buildScene()} />)
      expect(container).toBeEmptyDOMElement()
    })

    it('never renders a 0', () => {
      mockUseSceneNewArtists.mockReturnValue({ data: { artists: [] } })
      const { container } = renderWithProviders(<SceneNewBands scene={buildScene()} />)
      expect(container.textContent).not.toContain('0')
    })

    it('renders nothing while the request is in flight', () => {
      mockUseSceneNewArtists.mockReturnValue({ data: undefined })
      const { container } = renderWithProviders(<SceneNewBands scene={buildScene()} />)
      expect(container).toBeEmptyDOMElement()
    })
  })

  it('reads the scene canonical slug, not the requested one', () => {
    givenNewBands([band()])
    renderWithProviders(<SceneNewBands scene={buildScene()} />)
    expect(mockUseSceneNewArtists).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'phoenix-az' })
    )
  })

  it('uses no em dashes', () => {
    givenNewBands([band()])
    const { container } = renderWithProviders(<SceneNewBands scene={buildScene()} />)
    expect(container.textContent).not.toContain('—')
  })
})
