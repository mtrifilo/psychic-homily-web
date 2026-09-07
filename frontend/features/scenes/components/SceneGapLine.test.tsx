import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneDetail, SceneGapsResponse } from '../types'

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

const mockUseSceneGaps = vi.fn()
vi.mock('../hooks', () => ({
  useSceneGaps: (slug: unknown) => mockUseSceneGaps(slug),
}))

import { SceneGapLine } from './SceneGapLine'

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

function gaps(overrides: Partial<SceneGapsResponse> = {}): SceneGapsResponse {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    artists_missing_listen_link: 11,
    artists_on_bills_missing_location: 4,
    ...overrides,
  }
}

function mockGaps(data: SceneGapsResponse | undefined) {
  mockUseSceneGaps.mockReturnValue({ data })
}

beforeEach(() => {
  mockUseSceneGaps.mockReset()
})

describe('SceneGapLine', () => {
  it('keys the hook by slug', () => {
    mockGaps(gaps())
    renderWithProviders(<SceneGapLine scene={buildScene()} />)

    expect(mockUseSceneGaps).toHaveBeenCalledWith('phoenix-az')
  })

  it('states the gap and links to the artist list filtered to the scene city', () => {
    mockGaps(gaps())
    renderWithProviders(<SceneGapLine scene={buildScene()} />)

    expect(
      screen.getByRole('link', {
        name: '11 Phoenix bands have no listen link → Help finish Phoenix',
      })
    ).toHaveAttribute('href', '/artists?cities=Phoenix%2CAZ')
  })

  // The count is a fact about the place; the sentence names the page the
  // reader is on, not the city the gaps payload echoes back.
  it('names the scene city, not the payload city', () => {
    mockGaps(gaps({ city: 'Tempe' }))
    renderWithProviders(<SceneGapLine scene={buildScene()} />)

    const link = screen.getByRole('link')
    expect(link.textContent).toContain('Phoenix')
    expect(link.textContent).not.toContain('Tempe')
  })

  it('hides at zero', () => {
    mockGaps(gaps({ artists_missing_listen_link: 0 }))
    const { container } = renderWithProviders(
      <SceneGapLine scene={buildScene()} />
    )

    expect(container).toBeEmptyDOMElement()
  })

  // Loading and error both arrive as absent data, and neither may flash a
  // request for help.
  it('hides while there is no payload', () => {
    mockGaps(undefined)
    const { container } = renderWithProviders(
      <SceneGapLine scene={buildScene()} />
    )

    expect(container).toBeEmptyDOMElement()
  })

  // The payload's other count is over a different population and can never
  // switch this line on.
  it('ignores the missing-location count', () => {
    mockGaps(
      gaps({
        artists_missing_listen_link: 0,
        artists_on_bills_missing_location: 42,
      })
    )
    const { container } = renderWithProviders(
      <SceneGapLine scene={buildScene()} />
    )

    expect(container).toBeEmptyDOMElement()
  })
})
