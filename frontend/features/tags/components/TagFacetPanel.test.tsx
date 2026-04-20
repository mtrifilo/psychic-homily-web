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

vi.mock('../hooks', () => ({
  useTags: (params: { category?: keyof typeof tagsByCategory }) => ({
    data: params?.category ? { tags: tagsByCategory[params.category] ?? [], total: (tagsByCategory[params.category] ?? []).length } : { tags: [], total: 0 },
    isLoading: false,
  }),
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
})
