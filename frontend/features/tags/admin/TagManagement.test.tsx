import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type {
  TagAlias,
  TagAliasListing,
  TagDetailResponse,
  TagListItem,
} from '../types'

const mockUseTags = vi.fn()
vi.mock('../hooks', () => ({
  useTags: (...args: unknown[]) => mockUseTags(...args),
  useTag: vi.fn(),
  // MergeTagDialog (mounted inside TagManagement) pulls these in even when
  // the merge dialog is closed, so they have to exist in the mock.
  useSearchTags: () => ({ data: { tags: [] as TagListItem[] }, isLoading: false }),
  useTagAliases: () => ({ data: { aliases: [] as TagAlias[] }, isLoading: false }),
}))

vi.mock('./useAdminTags', () => ({
  useCreateTag: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateTag: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteTag: () => ({ mutate: vi.fn(), isPending: false }),
  useTagAliases: () => ({ data: { aliases: [] as TagAlias[] }, isLoading: false }),
  useCreateAlias: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteAlias: () => ({ mutate: vi.fn(), isPending: false }),
  useAllTagAliases: () => ({ data: { aliases: [] as TagAliasListing[], total: 0 }, isLoading: false, error: null as Error | null }),
  useBulkImportAliases: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTags: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTagsPreview: () => ({ data: null as unknown, isLoading: false, error: null as Error | null }),
  useLowQualityTagQueue: () => ({ data: { tags: [] as TagListItem[], total: 0 }, isLoading: false, error: null as Error | null }),
  useSnoozeTag: () => ({ mutate: vi.fn(), isPending: false, variables: undefined as unknown }),
  useMarkTagOfficial: () => ({ mutate: vi.fn(), isPending: false, variables: undefined as unknown }),
  useGenreHierarchy: () => ({ data: { tags: [] as TagListItem[] }, isLoading: false, error: null as Error | null }),
  useSetTagParent: () => ({ mutate: vi.fn(), isPending: false }),
}))

import { TagManagement, EditTagFormFields } from './TagManagement'

function makeTag(overrides: Partial<TagListItem> = {}): TagListItem {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 42,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeTagDetail(
  overrides: Partial<TagDetailResponse> = {}
): TagDetailResponse {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 42,
    created_at: '2025-01-01T00:00:00Z',
    description: 'A genre.',
    child_count: 0,
    aliases: [],
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('TagManagement — official indicator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the shared TagOfficialIndicator on official rows', () => {
    mockUseTags.mockReturnValue({
      data: {
        tags: [
          makeTag({ id: 1, name: 'shoegaze', slug: 'shoegaze', is_official: true }),
          makeTag({ id: 2, name: 'indie', slug: 'indie', is_official: false }),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagManagement />)

    const markers = screen.getAllByRole('img', { name: 'Official tag' })
    expect(markers).toHaveLength(1)
    expect(markers[0]).toHaveAttribute('title', 'shoegaze (Official)')
  })

  it('does not render the indicator when no tags are official', () => {
    mockUseTags.mockReturnValue({
      data: {
        tags: [makeTag({ is_official: false })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagManagement />)

    expect(screen.queryByRole('img', { name: 'Official tag' })).not.toBeInTheDocument()
  })
})

describe('TagManagement — tag count line (PSY-1103)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not render a stray "0" when total is 0 but rows are present', () => {
    // PSY-1103: `{tagsData?.total && ...}` rendered the literal 0 (a valid
    // React child) whenever the server-reported total was 0 while the tag
    // list still held rows — e.g. a stale/desynced cached page. The fix
    // leads with the comparison so the guard is a boolean, not a number.
    mockUseTags.mockReturnValue({
      data: { tags: [makeTag({ id: 1, name: 'rock' })], total: 0 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagManagement />)

    const countLine = screen.getByText(/1 tag/)
    // Before the fix this rendered "1 tag0" — the literal 0 from the falsy
    // left operand. Exact-text equality is the load-bearing assertion: a
    // looser /\b0\b/ match misses it because there is no word boundary
    // between "tag" and the appended "0".
    expect(countLine.textContent).toBe('1 tag')
    expect(countLine).not.toHaveTextContent(/of .* total/)
  })

  it('renders the "(of N total)" suffix when total exceeds visible rows', () => {
    mockUseTags.mockReturnValue({
      data: {
        tags: [makeTag({ id: 1, name: 'rock' })],
        total: 50,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagManagement />)

    expect(screen.getByText(/\(of 50 total\)/)).toBeInTheDocument()
  })
})

describe('TagManagement — category filter (PSY-924)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
    })
  })

  it('renders the category filter as the DS Select with the All sentinel', () => {
    // PSY-924: native <select> filter is now a Radix combobox; "All
    // Categories" is the FILTER_SELECT_ALL sentinel round-tripped to '' for
    // the query.
    renderWithProviders(<TagManagement />)
    const categorySelect = screen.getByRole('combobox', {
      name: 'Filter by category',
    })
    expect(categorySelect).toHaveTextContent('All Categories')
  })

  it('passes the selected category filter through to useTags', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagManagement />)

    await user.click(
      screen.getByRole('combobox', { name: 'Filter by category' })
    )
    await user.click(await screen.findByRole('option', { name: 'Genre' }))

    expect(mockUseTags).toHaveBeenLastCalledWith(
      expect.objectContaining({ category: 'genre' })
    )
  })

  it('round-trips the All sentinel back to no category filter', async () => {
    // Guards the sentinel: "All Categories" must clear category ('' →
    // undefined), not pass the literal 'all' to the backend query.
    const user = userEvent.setup()
    renderWithProviders(<TagManagement />)

    await user.click(
      screen.getByRole('combobox', { name: 'Filter by category' })
    )
    await user.click(await screen.findByRole('option', { name: 'Genre' }))
    expect(mockUseTags).toHaveBeenLastCalledWith(
      expect.objectContaining({ category: 'genre' })
    )

    await user.click(
      screen.getByRole('combobox', { name: 'Filter by category' })
    )
    await user.click(await screen.findByRole('option', { name: 'All Categories' }))
    expect(mockUseTags).toHaveBeenLastCalledWith(
      expect.objectContaining({ category: undefined })
    )
  })
})

describe('EditTagFormFields: tag switch resets fields via key prop', () => {
  // Pins PSY-768: the inner form initializes local state from the tag prop
  // on mount, with no useEffect and no `initialized` ratchet. Callers pass
  // `key={tag.id}` so React unmounts + remounts with fresh state when the
  // tag switches. The two assertions below are the load-bearing pair —
  // without both, a future maintainer could re-add a tag-prop-based reset
  // and the tests would still pass.

  it('resets fields when re-rendered with a different tag (via key prop)', async () => {
    const user = userEvent.setup()
    const tagA = makeTagDetail({ id: 1, name: 'rock', description: 'A' })
    const tagB = makeTagDetail({
      id: 2,
      name: 'jazz',
      description: 'B',
      category: 'genre',
    })

    const { rerender } = renderWithProviders(
      <EditTagFormFields
        key={tagA.id}
        tag={tagA}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    const nameInput = screen.getByLabelText('Name *')
    expect(nameInput).toHaveValue('rock')

    await user.clear(nameInput)
    await user.type(nameInput, 'dirty-edit')
    expect(nameInput).toHaveValue('dirty-edit')

    rerender(
      <EditTagFormFields
        key={tagB.id}
        tag={tagB}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Name *')).toHaveValue('jazz')
    expect(screen.getByLabelText('Description')).toHaveValue('B')
  })

  it('preserves dirty edits when re-rendered with the same key', async () => {
    const user = userEvent.setup()
    const tag = makeTagDetail({ id: 1, name: 'rock' })

    const { rerender } = renderWithProviders(
      <EditTagFormFields
        key={tag.id}
        tag={tag}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    const nameInput = screen.getByLabelText('Name *')
    await user.clear(nameInput)
    await user.type(nameInput, 'dirty-edit')

    rerender(
      <EditTagFormFields
        key={tag.id}
        tag={tag}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Name *')).toHaveValue('dirty-edit')
  })
})
