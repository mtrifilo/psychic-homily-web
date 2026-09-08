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

/**
 * Arrival order is deliberately neither the show-count order nor the name
 * order, so a client-side re-sort by either one fails the order case below.
 * The real endpoint ranks by count then name; the component's contract is to
 * print what it was handed, whatever that is.
 */
const CREWS: SceneCrewSummary[] = [
  { slug: 'relax-attack-jazz-series', name: 'Relax Attack Jazz Series', show_count: 1 },
  { slug: 'pleiades-series', name: 'Pleiades Series', show_count: 4 },
]

/** `null` is the absent payload, which is how both loading and error arrive. */
function renderCrews(crews: SceneCrewSummary[] | null = CREWS) {
  mockUseSceneCrews.mockReturnValue({
    data: crews === null ? undefined : { crews },
  })
  return renderWithProviders(<SceneCrews scene={buildScene()} />)
}

beforeEach(() => {
  mockUseSceneCrews.mockReset()
})

describe('SceneCrews', () => {
  it('keys the hook by slug', () => {
    renderCrews()

    expect(mockUseSceneCrews).toHaveBeenCalledWith('phoenix-az')
  })

  it('links each chip to the crew tag page', () => {
    renderCrews()

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
    renderCrews()

    const names = screen
      .getAllByRole('listitem')
      .map(item => item.textContent)
    expect(names).toEqual(['Relax Attack Jazz Series', 'Pleiades Series'])
  })

  // No cap: a crowded scene names every crew booking in it.
  it('renders every crew the endpoint returns', () => {
    const many = Array.from({ length: 14 }, (_, i) => ({
      slug: `crew-${i}`,
      name: `Crew ${i}`,
      show_count: 14 - i,
    }))
    renderCrews(many)

    expect(screen.getAllByRole('listitem')).toHaveLength(14)
  })

  // The mock's row WRAPS rather than truncating, and a name too long for the
  // column breaks inside its chip instead of widening the page. Neither comes
  // from the shared category treatment, so neither is covered by the classes
  // case below.
  it('wraps the row and breaks a name too long for the column', () => {
    renderCrews()

    expect(screen.getByRole('list')).toHaveClass('flex-wrap')
    const chip = screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    expect(chip).toHaveClass('break-words')
    expect(chip).toHaveClass('max-w-full')
    expect(chip).not.toHaveClass('truncate')
  })

  it('wears the crew category chip treatment', () => {
    renderCrews()

    const chip = screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    for (const cls of getCategoryChipClasses('crew').split(' ')) {
      expect(chip).toHaveClass(cls)
    }
  })

  // The show counts rank the row; they are not the reader's business. The
  // fixture's counts are digits no crew name in it contains, so a chip that
  // printed its rank would show up here.
  it('never prints the ranking counts', () => {
    const { container } = renderCrews()

    for (const crew of CREWS) {
      expect(container.textContent).not.toContain(String(crew.show_count))
    }
  })

  // The wire type is `crews: SceneCrewSummary[] | null`, which the local
  // response type narrows away; the guard has to hold against the shape the
  // API actually allows.
  it('hides when the payload carries a null list', () => {
    mockUseSceneCrews.mockReturnValue({ data: { crews: null } })
    const { container } = renderWithProviders(<SceneCrews scene={buildScene()} />)

    expect(container).toBeEmptyDOMElement()
  })

  it('hides completely when the scene has no crew tags', () => {
    const { container } = renderCrews([])

    expect(container).toBeEmptyDOMElement()
  })

  // Loading and error both arrive as absent data, and neither may flash a row
  // the payload may not warrant.
  it('hides while there is no payload', () => {
    const { container } = renderCrews(null)

    expect(container).toBeEmptyDOMElement()
  })

  // A slug that resolves to the tag INDEX rather than to the crew: the chip is
  // still named, and it is not a link.
  it('names a crew it cannot link, without linking it', () => {
    renderCrews([{ slug: '', name: 'Unslugged Crew', show_count: 2 }])

    const item = screen.getByRole('listitem')
    expect(item).toHaveTextContent('Unslugged Crew')
    expect(within(item).queryByRole('link')).not.toBeInTheDocument()
  })

  // City AND state, the spelling every other identity string on the page uses:
  // the catalog carries same-named cities in different states.
  it('names the row after the scene the page is on', () => {
    renderCrews()

    expect(
      screen.getByRole('list', { name: 'Crews booking in Phoenix, AZ' })
    ).toBeInTheDocument()
  })

  // `list-style: none` costs a `ul` its list semantics in WebKit, and a
  // generic element drops the label with them, which is invisible to jsdom.
  it('carries an explicit list role', () => {
    renderCrews()

    expect(screen.getByRole('list')).toHaveAttribute('role', 'list')
  })
})
