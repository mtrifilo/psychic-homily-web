import { describe, it, expect, vi } from 'vitest'
import { screen, within } from '@testing-library/react'
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

// City-scoped overrides (PSY-982): when entity_type=show AND cities are passed,
// some tags drop to 0 because the selected city has no matching shows. shoegaze
// keeps a non-zero count so we can assert a mix of enabled + disabled chips.
const cityScopedShowCounts: Record<string, number> = {
  'post-punk': 4,
  shoegaze: 0,
  phoenix: 0,
  diy: 0,
}

vi.mock('../hooks', () => ({
  useTags: (params: {
    category?: keyof typeof tagsByCategory
    entity_type?: string
    cities?: Array<{ city: string; state: string }>
  }) => {
    const baseTags = params?.category ? tagsByCategory[params.category] ?? [] : []
    const cityScoped =
      params?.entity_type === 'show' && (params?.cities?.length ?? 0) > 0
    const tags = cityScoped
      ? baseTags.map(t => ({
          ...t,
          usage_count: cityScopedShowCounts[t.slug] ?? 0,
        }))
      : params?.entity_type
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

  it('shows the multi-tag selection tooltip copy for the shows facet', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="show"
      />
    )
    await user.hover(screen.getByTestId('tag-facet-transitive-info'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'Select one or more tags to filter shows based on any tag combination.'
    )
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

  // PSY-982: city-scoped /shows facet. When a city is selected, zero-in-city
  // chips are SHOWN but DISABLED (not hidden) so they can't dead-end at
  // "0 shows", and the non-zero chips stay clickable.
  describe('city-scoped show facet (PSY-982)', () => {
    const cities = [{ city: 'Phoenix', state: 'AZ' }]

    it('shows zero-in-city chips disabled instead of hiding them', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="show"
          selectedCities={cities}
        />
      )
      // post-punk has shows in Phoenix → enabled.
      const postPunk = screen.getByTestId('tag-facet-chip-post-punk')
      expect(postPunk).toBeInTheDocument()
      expect(postPunk).not.toBeDisabled()
      // shoegaze / phoenix / diy have 0 Phoenix shows → present but disabled.
      const shoegaze = screen.getByTestId('tag-facet-chip-shoegaze')
      expect(shoegaze).toBeInTheDocument()
      expect(shoegaze).toBeDisabled()
      expect(screen.getByTestId('tag-facet-chip-phoenix')).toBeDisabled()
      expect(screen.getByTestId('tag-facet-chip-diy')).toBeDisabled()
    })

    it('does not call onToggle when a disabled zero-in-city chip is clicked', async () => {
      const user = userEvent.setup()
      const onToggle = vi.fn()
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={onToggle}
          onClear={() => {}}
          entityType="show"
          selectedCities={cities}
        />
      )
      await user.click(screen.getByTestId('tag-facet-chip-shoegaze'))
      expect(onToggle).not.toHaveBeenCalled()
    })

    it('omits the "Show all tags" expander in city-scoped mode', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="show"
          selectedCities={cities}
        />
      )
      expect(screen.queryByTestId('tag-facet-show-all')).not.toBeInTheDocument()
    })

    it('keeps a selected zero-in-city chip interactive so it can be deselected', async () => {
      const user = userEvent.setup()
      const onToggle = vi.fn()
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={['shoegaze']}
          onToggle={onToggle}
          onClear={() => {}}
          entityType="show"
          selectedCities={cities}
        />
      )
      const shoegaze = screen.getByTestId('tag-facet-chip-shoegaze')
      expect(shoegaze).not.toBeDisabled()
      await user.click(shoegaze)
      expect(onToggle).toHaveBeenCalledWith([])
    })

    it('falls back to the global hide-behavior when no city is selected', () => {
      // entityType=show with no cities → not city-scoped → uses the PSY-484
      // path. With the default stub counts every show chip is non-zero, so all
      // chips render enabled and the expander reappears only if something is
      // hidden (nothing is here).
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="show"
        />
      )
      expect(screen.getByTestId('tag-facet-chip-post-punk')).not.toBeDisabled()
      expect(screen.getByTestId('tag-facet-chip-shoegaze')).not.toBeDisabled()
    })
  })

  // PSY-499: transitive filter info tooltip. Only rendered for show/festival
  // because those are the container entity types whose genre meaning comes
  // from their lineup artists — direct-tag pages (artist, venue, label,
  // release) don't need the explainer.
  it('renders the transitive-filter info tooltip trigger for show entityType', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="show"
      />
    )
    const info = screen.getByTestId('tag-facet-transitive-info')
    expect(info).toBeInTheDocument()
    expect(info).toHaveAttribute('aria-label', 'How tag filtering works')
  })

  it('renders the transitive-filter info tooltip trigger for festival entityType', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="festival"
      />
    )
    expect(screen.getByTestId('tag-facet-transitive-info')).toBeInTheDocument()
  })

  it('omits the transitive tooltip on direct-tag entity types', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="artist"
      />
    )
    expect(
      screen.queryByTestId('tag-facet-transitive-info')
    ).not.toBeInTheDocument()
  })

  it('omits the transitive tooltip when no entityType is provided', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
      />
    )
    expect(
      screen.queryByTestId('tag-facet-transitive-info')
    ).not.toBeInTheDocument()
  })

  it('omits the transitive tooltip when the heading is hidden (sheet mode)', () => {
    renderWithProviders(
      <TagFacetPanel
        selectedSlugs={[]}
        onToggle={() => {}}
        onClear={() => {}}
        entityType="show"
        hideHeading
      />
    )
    // Sheet mode lifts the heading to a SheetTitle; the tooltip lives with
    // the heading so it goes away too. (The sheet's own title handles mobile
    // discovery if a mobile tooltip is added later.)
    expect(
      screen.queryByTestId('tag-facet-transitive-info')
    ).not.toBeInTheDocument()
  })

  // PSY-1000: horizontal top-bar layout. Same data + behavior as the rail
  // default — chips, live counts, selection, disabled zero-result facets,
  // and the "show all tags" expander all carry over — only the container
  // changes (one wrapping row above a full-width list instead of a sidebar).
  describe('bar layout (PSY-1000)', () => {
    it('defaults to the rail layout when no layout prop is given', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
        />
      )
      expect(screen.getByTestId('tag-facet-panel')).toHaveAttribute(
        'data-layout',
        'rail'
      )
    })

    it('renders all categories and chips in bar layout', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          layout="bar"
        />
      )
      expect(screen.getByTestId('tag-facet-panel')).toHaveAttribute(
        'data-layout',
        'bar'
      )
      // Every category's chips still render (flowed into one row).
      expect(screen.getByTestId('tag-facet-chip-post-punk')).toBeInTheDocument()
      expect(screen.getByTestId('tag-facet-chip-phoenix')).toBeInTheDocument()
      expect(screen.getByTestId('tag-facet-chip-diy')).toBeInTheDocument()
    })

    it('toggles a chip in bar layout', async () => {
      const user = userEvent.setup()
      const onToggle = vi.fn()
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={['shoegaze']}
          onToggle={onToggle}
          onClear={() => {}}
          layout="bar"
        />
      )
      await user.click(screen.getByTestId('tag-facet-chip-post-punk'))
      expect(onToggle).toHaveBeenCalledWith(['shoegaze', 'post-punk'])
    })

    it('shows the Clear all button in bar layout when there is a selection', async () => {
      const user = userEvent.setup()
      const onClear = vi.fn()
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={['post-punk']}
          onToggle={() => {}}
          onClear={onClear}
          layout="bar"
        />
      )
      const clearBtn = screen.getByTestId('tag-facet-clear')
      await user.click(clearBtn)
      expect(onClear).toHaveBeenCalled()
    })

    it('renders the "Show all tags" expander in bar layout', async () => {
      const user = userEvent.setup()
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="festival"
          layout="bar"
        />
      )
      // shoegaze is hidden (0 festival applications) until the expander fires.
      expect(
        screen.queryByTestId('tag-facet-chip-shoegaze')
      ).not.toBeInTheDocument()
      const expander = screen.getByTestId('tag-facet-show-all')
      await user.click(expander)
      expect(screen.getByTestId('tag-facet-chip-shoegaze')).toBeInTheDocument()
    })

    it('disables zero-in-city chips in bar layout (city-scoped show facet)', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="show"
          selectedCities={[{ city: 'Phoenix', state: 'AZ' }]}
          layout="bar"
        />
      )
      // post-punk has Phoenix shows → enabled; shoegaze has 0 → disabled.
      expect(screen.getByTestId('tag-facet-chip-post-punk')).not.toBeDisabled()
      expect(screen.getByTestId('tag-facet-chip-shoegaze')).toBeDisabled()
      // City-scoped mode omits the expander in bar layout too.
      expect(
        screen.queryByTestId('tag-facet-show-all')
      ).not.toBeInTheDocument()
    })

    it('renders the transitive info tooltip in bar layout for show entityType', () => {
      renderWithProviders(
        <TagFacetPanel
          selectedSlugs={[]}
          onToggle={() => {}}
          onClear={() => {}}
          entityType="show"
          layout="bar"
        />
      )
      expect(screen.getByTestId('tag-facet-transitive-info')).toBeInTheDocument()
    })
  })
})
