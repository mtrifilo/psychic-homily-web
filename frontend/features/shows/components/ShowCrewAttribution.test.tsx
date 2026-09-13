import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { getCategoryChipClasses } from '@/features/tags/types'
import type { EntityTag } from '@/features/tags/types'

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

const mockUseEntityTags = vi.fn()
vi.mock('@/features/tags/hooks', () => ({
  useEntityTags: (entityType: unknown, entityId: unknown) =>
    mockUseEntityTags(entityType, entityId),
}))

import { ShowCrewAttribution } from './ShowCrewAttribution'

let nextTagId = 100

function tag(overrides: Partial<EntityTag> & Pick<EntityTag, 'name'>): EntityTag {
  return {
    tag_id: nextTagId++,
    slug: overrides.name.trim().toLowerCase().replace(/\s+/g, '-'),
    category: 'crew',
    is_official: false,
    upvotes: 0,
    downvotes: 0,
    wilson_score: 0,
    ...overrides,
  }
}

const RELAX = tag({ tag_id: 11, name: 'Relax Attack Jazz Series' })
const PLEIADES = tag({ tag_id: 12, name: 'Pleiades Series' })
const GENRE = tag({ tag_id: 20, name: 'post-punk', category: 'genre' })
const LOCALE = tag({ tag_id: 21, name: 'chicago', category: 'locale' })

/** `undefined` data is how both loading and error arrive from the query. */
function renderRow(tags: EntityTag[] | undefined) {
  mockUseEntityTags.mockReturnValue({ data: tags === undefined ? undefined : { tags } })
  return renderWithProviders(<ShowCrewAttribution showId={42} />)
}

beforeEach(() => {
  mockUseEntityTags.mockReset()
})

describe('ShowCrewAttribution', () => {
  it('reads the show tag query the tag list below it reads', () => {
    renderRow([RELAX])

    expect(mockUseEntityTags).toHaveBeenCalledWith('show', 42)
  })

  it('names the row and links each chip to the crew tag page', () => {
    renderRow([GENRE, RELAX, PLEIADES])

    expect(screen.getByText('Presented by')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    ).toHaveAttribute('href', '/tags/relax-attack-jazz-series')
    expect(screen.getByRole('link', { name: 'Pleiades Series' })).toHaveAttribute(
      'href',
      '/tags/pleiades-series'
    )
  })

  // The label names the list rather than sitting beside it unattached, so a
  // screen reader reaching the chips is told what they credit.
  it('labels the chip list with the visible label', () => {
    renderRow([RELAX])

    expect(screen.getByRole('list', { name: 'Presented by' })).toBeInTheDocument()
  })

  // Descriptive tags belong to the tag list below; drawing them here would
  // credit a genre as a booker.
  it('draws only the crew-category tags', () => {
    renderRow([GENRE, LOCALE, RELAX])

    expect(screen.getAllByRole('listitem')).toHaveLength(1)
    expect(screen.queryByText('post-punk')).not.toBeInTheDocument()
    expect(screen.queryByText('chicago')).not.toBeInTheDocument()
  })

  // `tags.category` is an unconstrained column, so the guard has to normalize.
  it('recognizes a crew tag stored under a different casing', () => {
    renderRow([tag({ tag_id: 13, name: 'Pleiades Series', category: 'Crew' })])

    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })

  // `ListEntityTags` issues no ORDER BY, so arrival order is not an order.
  // Every score here is zero, which is the common case, and the fixture
  // arrives in neither alphabetical nor reverse-alphabetical order.
  it('orders the chips deterministically when no crew has votes', () => {
    renderRow([RELAX, PLEIADES])

    expect(screen.getAllByRole('listitem').map(item => item.textContent)).toEqual([
      'Pleiades Series',
      'Relax Attack Jazz Series',
    ])
  })

  it('leads with the most-upvoted crew', () => {
    renderRow([
      PLEIADES,
      tag({ tag_id: 11, name: 'Relax Attack Jazz Series', wilson_score: 0.6 }),
    ])

    expect(screen.getAllByRole('listitem')[0]).toHaveTextContent(
      'Relax Attack Jazz Series'
    )
  })

  // A tag whose slug addresses the tag INDEX rather than the tag is still
  // named: the name is what the chip is for.
  it.each(['', '   ', '.', '..'])(
    'names but does not link a crew whose slug is %p',
    slug => {
      renderRow([tag({ tag_id: 14, name: 'Pleiades Series', slug })])

      expect(screen.getByText('Pleiades Series')).toBeInTheDocument()
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    }
  )

  it('encodes a slug that would otherwise splice a second path segment', () => {
    renderRow([tag({ tag_id: 15, name: 'Pleiades Series', slug: 'a/b' })])

    expect(screen.getByRole('link', { name: 'Pleiades Series' })).toHaveAttribute(
      'href',
      '/tags/a%2Fb'
    )
  })

  it('drops a crew it cannot name', () => {
    renderRow([tag({ tag_id: 16, name: '   ', slug: 'nameless' }), RELAX])

    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })

  // The mock's row wraps rather than truncating, and a name too long for the
  // column breaks inside its chip instead of widening the page.
  it('wraps the row and breaks a name too long for the column', () => {
    renderRow([RELAX])

    expect(screen.getByRole('list')).toHaveClass('flex-wrap')
    const chip = screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    expect(chip).toHaveClass('break-words')
    expect(chip).toHaveClass('max-w-full')
    expect(chip).not.toHaveClass('truncate')
  })

  it('wears the crew category chip treatment', () => {
    renderRow([RELAX])

    const chip = screen.getByRole('link', { name: 'Relax Attack Jazz Series' })
    for (const cls of getCategoryChipClasses('crew').split(' ')) {
      expect(chip).toHaveClass(cls)
    }
  })

  // The row is absent, not empty-stated: most shows have no crew tag, and a
  // label over nothing reports a gap in the catalog as a fact about the show.
  it('hides on a show with no crew tag', () => {
    const { container } = renderRow([GENRE, LOCALE])

    expect(container).toBeEmptyDOMElement()
  })

  it('hides on a show with no tags at all', () => {
    const { container } = renderRow([])

    expect(container).toBeEmptyDOMElement()
  })

  // Loading and error both arrive as absent data, and neither may flash a
  // credit the payload may not warrant.
  it('hides while there is no payload', () => {
    const { container } = renderRow(undefined)

    expect(container).toBeEmptyDOMElement()
  })
})
