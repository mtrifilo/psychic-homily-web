import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneCollectionSummary, SceneDetail } from '../types'

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

const mockUseSceneCollections = vi.fn()
vi.mock('../hooks', () => ({
  useSceneCollections: (options: unknown) => mockUseSceneCollections(options),
}))

import { SceneCollections } from './SceneCollections'

const NOW = new Date('2026-09-07T12:00:00Z')

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

function collection(
  overrides: Partial<SceneCollectionSummary> = {}
): SceneCollectionSummary {
  return {
    id: 1,
    slug: 'phoenix-diy-essentials',
    title: 'Phoenix DIY Essentials',
    cover_image_url: null,
    scene_local_item_count: 12,
    item_count: 14,
    contributor_count: 4,
    updated_at: '2026-09-04T12:00:00Z',
    ...overrides,
  }
}

function renderRail(collections: SceneCollectionSummary[]) {
  mockUseSceneCollections.mockReturnValue({ data: { collections } })
  return renderWithProviders(<SceneCollections scene={buildScene()} />)
}

beforeEach(() => {
  mockUseSceneCollections.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

/** Called by the tests that read the meta line, which prints a relative time. */
function freezeClock() {
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
}

describe('SceneCollections', () => {
  it('heads the rail with the city', () => {
    renderRail([collection()])
    expect(
      screen.getByRole('heading', { level: 2, name: /Collections · Phoenix/ })
    ).toBeInTheDocument()
  })

  it('renders one row per collection, in the order the payload gave', () => {
    renderRail([
      collection({ id: 1, title: 'Phoenix DIY Essentials' }),
      collection({
        id: 2,
        slug: 'trunk-space-regulars',
        title: 'Trunk Space Regulars',
      }),
    ])
    const rows = screen.getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('Phoenix DIY Essentials')
    expect(rows[1]).toHaveTextContent('Trunk Space Regulars')
  })

  // The whole row is the target, so the link's accessible name is pinned
  // exactly: without the explicit label it would be the row's full text, which
  // puts a middot and a relative timestamp inside a link name.
  it('links each row to the collection under the title as its name', () => {
    freezeClock()
    renderRail([collection()])
    const link = screen.getByRole('link', { name: 'Phoenix DIY Essentials' })
    expect(link).toHaveAttribute('href', '/collections/phoenix-diy-essentials')
    expect(
      screen.queryByRole('link', { name: /Updated|·/ })
    ).not.toBeInTheDocument()
  })

  // `/collections/..` walks back up to the browse index, the same wrong
  // destination an empty slug produces.
  it('does not link a dot-segment slug', () => {
    renderRail([collection({ slug: '..' })])
    expect(screen.getByText('Phoenix DIY Essentials')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  // A blank slug resolves `/collections/` to the browse index, not a 404, so an
  // unlinkable row must still be named rather than sent to a directory that
  // never mentions it.
  it('names a slugless collection without linking it', () => {
    renderRail([collection({ slug: '' })])
    expect(screen.getByText('Phoenix DIY Essentials')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('prints the builder count and the edit recency', () => {
    freezeClock()
    renderRail([
      collection({ contributor_count: 4, updated_at: '2026-09-04T12:00:00Z' }),
    ])
    expect(screen.getByText('Built by 4 · Updated 3 days ago')).toBeInTheDocument()
  })

  // The weeks phrasing is the whole reason this module reads formatTimeAgo
  // rather than the formatRelativeTime its sibling collections card uses: the
  // other formatter jumps from days straight to an absolute date, which the
  // mock's `UPDATED 1 WEEK AGO` is not.
  it('words a fortnight-old edit in weeks, not as a date', () => {
    freezeClock()
    renderRail([collection({ updated_at: '2026-08-24T12:00:00Z' })])
    expect(screen.getByText(/Updated 2 weeks ago/)).toBeInTheDocument()
  })

  // "Built by 0" is a claim about who assembled the collection that is false
  // however it arrives, so the clause drops rather than printing the zero.
  it('drops the builder clause when nobody is named', () => {
    freezeClock()
    renderRail([collection({ contributor_count: 0 })])
    expect(screen.getByText('Updated 3 days ago')).toBeInTheDocument()
    expect(screen.queryByText(/Built by/)).not.toBeInTheDocument()
  })

  it('renders the cover when the collection has one', () => {
    renderRail([
      collection({ cover_image_url: 'https://example.test/cover.jpg' }),
    ])
    const img = document.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://example.test/cover.jpg')
    // Empty alt: the title is adjacent, so a described cover would make the row
    // announce its name twice.
    expect(img).toHaveAttribute('alt', '')
  })

  it('draws the fallback tile, not a broken image, when there is no cover', () => {
    const { container } = renderRail([collection({ cover_image_url: null })])
    expect(document.querySelector('img')).toBeNull()
    expect(container.querySelector('svg.lucide-library')).toBeInTheDocument()
    expect(screen.getByText('Phoenix DIY Essentials')).toBeInTheDocument()
  })

  // The rail does not draw the qualifying counts the payload carries; they
  // audit the backend's ranking, they are not an offer to the reader. The
  // clock is frozen because the assertion is a substring search over the whole
  // row, and a live clock could put a "12" into the recency clause.
  it('prints neither the scene-local count nor the item count', () => {
    freezeClock()
    renderRail([collection({ scene_local_item_count: 12, item_count: 14 })])
    const row = screen.getByRole('listitem')
    expect(row).not.toHaveTextContent('12')
    expect(row).not.toHaveTextContent('14')
  })

  // Sibling convention: no em dashes anywhere in rendered scene copy.
  it('uses no em dashes', () => {
    freezeClock()
    const { container } = renderRail([collection()])
    expect(container.textContent).not.toContain('—')
  })

  it('renders nothing when no collection qualifies', () => {
    const { container } = renderRail([])
    expect(container).toBeEmptyDOMElement()
  })

  // The handler substitutes an empty slice, so the wire should never carry a
  // null. The generated response type still allows one, and a null must read
  // as "nothing to show" rather than throw on `.length`.
  it('renders nothing when the payload carries a null list', () => {
    mockUseSceneCollections.mockReturnValue({ data: { collections: null } })
    const { container } = renderWithProviders(
      <SceneCollections scene={buildScene()} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  // Loading and a 404 (a place below the scene venue threshold) both reach the
  // component as absent data, and neither may flash a heading over empty space.
  it('renders nothing when there is no data', () => {
    mockUseSceneCollections.mockReturnValue({ data: undefined })
    const { container } = renderWithProviders(
      <SceneCollections scene={buildScene()} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('asks the endpoint for the scene and passes no limit of its own', () => {
    renderRail([collection()])
    expect(mockUseSceneCollections).toHaveBeenCalledWith({ slug: 'phoenix-az' })
  })
})
