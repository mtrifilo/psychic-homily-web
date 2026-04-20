import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import {
  TagFacetPanel,
  parseTagsParam,
  buildTagsParam,
} from './TagFacetPanel'

// Stub the useTags hook so the panel renders a deterministic, per-category
// list without hitting the network. We return different tags per category
// so we can assert on cross-category selection.
//
// When entity_type is set, the per-tag usage_count is overridden to mimic
// the PSY-484 backend behavior: a tag may have a high global count but
// zero applications for the current entity type. The genre category is
// used as the canary — `shoegaze` returns 0 for `entity_type=festival`
// so we can assert the panel hides it by default and exposes it via the
// "Show all tags" expander.
const tagsByCategory = {
  genre: [
    { id: 1, slug: 'post-punk', name: 'post-punk', category: 'genre', is_official: false, usage_count: 42, created_at: '' },
    { id: 2, slug: 'shoegaze', name: 'shoegaze', category: 'genre', is_official: false, usage_count: 17, created_at: '' },
  ],
  locale: [
    { id: 3, slug: 'phoenix', name: 'phoenix', category: 'locale', is_official: false, usage_count: 80, created_at: '' },
  ],
  other: [
    { id: 4, slug: 'diy', name: 'diy', category: 'other', is_official: false, usage_count: 12, created_at: '' },
  ],
}

// Per-entity-type overrides: { entity_type: { tag_slug: count } }. Any tag
// not listed inherits its default usage_count above. Used to simulate the
// "this tag is popular globally but has zero matches on this browse page"
// scenario that PSY-484 fixes.
const entityTypeOverrides: Record<string, Record<string, number>> = {
  festival: { shoegaze: 0, phoenix: 0, diy: 0 },
}

vi.mock('../hooks', () => ({
  useTags: (params: { category?: keyof typeof tagsByCategory; entity_type?: string }) => {
    const baseTags = params?.category ? tagsByCategory[params.category] ?? [] : []
    const tags = params?.entity_type
      ? baseTags.map(t => {
          const override = entityTypeOverrides[params.entity_type ?? '']?.[t.slug]
          return override !== undefined ? { ...t, usage_count: override } : t
        })
      : baseTags
    return {
      data: { tags, total: tags.length },
      isLoading: false,
    }
  },
}))

describe('parseTagsParam / buildTagsParam', () => {
  it('parses comma-separated, trimmed, deduped slugs', () => {
    expect(parseTagsParam(' post-punk , post-punk, shoegaze,')).toEqual([
      'post-punk',
      'shoegaze',
    ])
    expect(parseTagsParam(null)).toEqual([])
    expect(parseTagsParam('')).toEqual([])
  })
  it('builds a comma-separated param', () => {
    expect(buildTagsParam(['a', 'b', 'c'])).toEqual('a,b,c')
  })
})

describe('TagFacetPanel', () => {
  it('renders all 3 categories with their tags', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
      />
    )
    expect(screen.getByTestId('tag-facet-category-genre')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-category-locale')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-category-other')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-post-punk')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-phoenix')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-diy')).toBeInTheDocument()
  })

  it('marks selected chips via aria-pressed', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={['shoegaze', 'phoenix']}
        onToggle={() => {}}
        onClear={() => {}}
      />
    )
    expect(screen.getByTestId('tag-facet-chip-shoegaze')).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByTestId('tag-facet-chip-phoenix')).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByTestId('tag-facet-chip-post-punk')).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  })

  it('calls onToggle with the appended slug when adding', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={['shoegaze']}
        onToggle={onToggle}
        onClear={() => {}}
      />
    )
    await user.click(screen.getByTestId('tag-facet-chip-post-punk'))
    expect(onToggle).toHaveBeenCalledWith(['shoegaze', 'post-punk'])
  })

  it('calls onToggle with the slug removed when deselecting', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={['post-punk', 'shoegaze']}
        onToggle={onToggle}
        onClear={() => {}}
      />
    )
    await user.click(screen.getByTestId('tag-facet-chip-post-punk'))
    expect(onToggle).toHaveBeenCalledWith(['shoegaze'])
  })

  it('shows the Clear all button only when there is a selection', async () => {
    const user = userEvent.setup()
    const onClear = vi.fn()
    const { rerender } = renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={onClear}
      />
    )
    expect(screen.queryByTestId('tag-facet-clear')).not.toBeInTheDocument()

    rerender(
      <TagFacetPanel
        selectedSlugs={['post-punk']}
        onToggle={() => {}}
        onClear={onClear}
      />
    )
    const clearBtn = screen.getByTestId('tag-facet-clear')
    await user.click(clearBtn)
    expect(onClear).toHaveBeenCalled()
  })

  it('supports cross-category selection', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={['post-punk']}
        onToggle={onToggle}
        onClear={() => {}}
      />
    )
    // Click a tag in the locale category while having a genre tag selected.
    const localeSection = screen.getByTestId('tag-facet-category-locale')
    await user.click(within(localeSection).getByTestId('tag-facet-chip-phoenix'))
    expect(onToggle).toHaveBeenCalledWith(['post-punk', 'phoenix'])
  })

  // PSY-484: when entityType is set, chips with usage_count=0 are hidden
  // by default. The "Show all tags" expander reveals them. Selected chips
  // remain visible regardless so the user can always deselect.
  it('hides zero-count chips when entityType is set', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="festival"
      />
    )
    // post-punk has 42 festival applications in this stub → visible.
    expect(screen.getByTestId('tag-facet-chip-post-punk')).toBeInTheDocument()
    // shoegaze, phoenix, and diy are all overridden to 0 for festival → hidden.
    expect(screen.queryByTestId('tag-facet-chip-shoegaze')).not.toBeInTheDocument()
    expect(screen.queryByTestId('tag-facet-chip-phoenix')).not.toBeInTheDocument()
    expect(screen.queryByTestId('tag-facet-chip-diy')).not.toBeInTheDocument()
    // The locale and other category headings hide entirely when their
    // categories collapse to zero visible chips — the panel only shows
    // categories with at least one chip.
    expect(screen.queryByTestId('tag-facet-category-locale')).not.toBeInTheDocument()
    expect(screen.queryByTestId('tag-facet-category-other')).not.toBeInTheDocument()
  })

  it('does not hide any chips when entityType is omitted', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
      />
    )
    // All four chips remain because the panel falls back to the global
    // usage_count when entityType is not set (e.g. /tags browse).
    expect(screen.getByTestId('tag-facet-chip-post-punk')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-shoegaze')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-phoenix')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-diy')).toBeInTheDocument()
    // No "Show all tags" expander when there's nothing to expand.
    expect(screen.queryByTestId('tag-facet-show-all')).not.toBeInTheDocument()
  })

  it('Show all tags expander reveals zero-count chips', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="festival"
      />
    )
    const expander = screen.getByTestId('tag-facet-show-all')
    expect(expander).toHaveTextContent(/show all tags/i)
    await user.click(expander)
    // After expansion, the previously-hidden zero-count chips appear.
    expect(screen.getByTestId('tag-facet-chip-shoegaze')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-phoenix')).toBeInTheDocument()
    expect(screen.getByTestId('tag-facet-chip-diy')).toBeInTheDocument()
    // The expander becomes a "Hide" toggle so the user can collapse again.
    expect(expander).toHaveTextContent(/hide tags with no matches/i)
  })

  it('keeps a selected zero-count chip visible even with the expander collapsed', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={['shoegaze']} // shoegaze has 0 festival applications
        onToggle={() => {}}
        onClear={() => {}}
        entityType="festival"
      />
    )
    // The selected chip stays visible so the user can deselect it without
    // hunting for the expander, even though its count is 0 for festivals.
    const shoegaze = screen.getByTestId('tag-facet-chip-shoegaze')
    expect(shoegaze).toBeInTheDocument()
    expect(shoegaze).toHaveAttribute('aria-pressed', 'true')
  })
})
