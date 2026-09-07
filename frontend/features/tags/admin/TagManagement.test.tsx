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
const mockUpdateTagMutate = vi.hoisted(() => vi.fn())
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
  useUpdateTag: () => ({ mutate: mockUpdateTagMutate, isPending: false }),
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

describe('TagManagement: crew category (PSY-1883)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTags.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
    })
  })

  it('offers Crew as an option when editing a tag that is not crew', async () => {
    // Asserting the trigger on a crew tag would pass off the unknown-category
    // fallback below rather than off the vocabulary.
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={7}
        tag={makeTagDetail({ id: 7, name: 'shoegaze', category: 'genre' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    await user.click(screen.getByLabelText('Category *'))
    expect(await screen.findByRole('option', { name: 'Crew' })).toBeInTheDocument()
  })

  it('offers an odd-cased stored category once, under the spelling the server takes', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={11}
        tag={makeTagDetail({ id: 11, name: 'Rubber Brother Records', category: 'Crew' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Category *')).toHaveTextContent('Crew')
    await user.click(screen.getByLabelText('Category *'))
    expect(await screen.findAllByRole('option', { name: 'Crew' })).toHaveLength(1)
  })

  it('shows a stored category the UI has never heard of rather than a blank trigger', async () => {
    // tags.category is an unconstrained column, so a seeder or a newer server
    // can store a value this build does not list.
    renderWithProviders(
      <EditTagFormFields
        key={8}
        tag={makeTagDetail({ id: 8, name: 'nineties', category: 'era' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Category *')).toHaveTextContent('Era')
  })

  it('renders rather than crashing for a tag stored with no category', async () => {
    // Radix rejects a SelectItem whose value is the empty string, so the
    // unknown-category option has to skip that one case.
    expect(() =>
      renderWithProviders(
        <EditTagFormFields
          key={9}
          tag={makeTagDetail({ id: 9, name: 'orphan', category: '' })}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )
    ).not.toThrow()
    expect(screen.getByLabelText('Name *')).toHaveValue('orphan')
  })

  it('offers Crew when filtering the admin tag list', async () => {
    const user = userEvent.setup()
    renderWithProviders(<TagManagement />)

    await user.click(
      screen.getByRole('combobox', { name: 'Filter by category' })
    )
    await user.click(await screen.findByRole('option', { name: 'Crew' }))

    expect(mockUseTags).toHaveBeenLastCalledWith(
      expect.objectContaining({ category: 'crew' })
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

describe('TagManagement — crew outbound links (PSY-1888)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('offers no link inputs for a non-crew tag', () => {
    renderWithProviders(
      <EditTagFormFields
        key={1}
        tag={makeTagDetail({ id: 1, name: 'shoegaze', category: 'genre' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.queryByLabelText('Website')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Instagram')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Bandcamp')).not.toBeInTheDocument()
  })

  it('offers the three link inputs for a crew tag, prefilled from the stored links', () => {
    renderWithProviders(
      <EditTagFormFields
        key={2}
        tag={makeTagDetail({
          id: 2,
          name: 'Rubber Brother Records',
          category: 'crew',
          social: {
            website: 'https://rubberbrotherrecords.test',
            instagram: 'https://instagram.com/rubberbrother',
            bandcamp: null,
          },
        })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Website')).toHaveValue(
      'https://rubberbrotherrecords.test'
    )
    expect(screen.getByLabelText('Instagram')).toHaveValue(
      'https://instagram.com/rubberbrother'
    )
    expect(screen.getByLabelText('Bandcamp')).toHaveValue('')
  })

  it('reveals the link inputs when the category is switched to crew', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={3}
        tag={makeTagDetail({ id: 3, name: 'shoegaze', category: 'genre' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.queryByLabelText('Website')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('Category *'))
    await user.click(await screen.findByRole('option', { name: 'Crew' }))

    expect(await screen.findByLabelText('Website')).toBeInTheDocument()
  })

  it('submits the trimmed link fields for a crew tag', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={4}
        tag={makeTagDetail({ id: 4, name: 'Rubber Brother', category: 'crew' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    await user.type(
      screen.getByLabelText('Website'),
      '  https://rubberbrotherrecords.test  '
    )
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mockUpdateTagMutate).toHaveBeenCalledTimes(1)
    const [payload] = mockUpdateTagMutate.mock.calls[0]
    expect(payload.tagId).toBe(4)
    expect(payload.data.website).toBe('https://rubberbrotherrecords.test')
    expect(payload.data.instagram).toBe('')
    expect(payload.data.bandcamp).toBe('')
  })

  // A form that does not show the inputs must not send them: stored links a
  // non-crew editor never saw would otherwise be cleared by saving a rename.
  it('sends no link fields at all for a non-crew tag with no stored links', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={5}
        tag={makeTagDetail({ id: 5, name: 'shoegaze', category: 'genre' })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mockUpdateTagMutate).toHaveBeenCalledTimes(1)
    const [payload] = mockUpdateTagMutate.mock.calls[0]
    expect(payload.data).not.toHaveProperty('website')
    expect(payload.data).not.toHaveProperty('instagram')
    expect(payload.data).not.toHaveProperty('bandcamp')
  })

  // Recategorizing a crew tag leaves its links stored and rendered. If the
  // only surface that can clear them hid them, they would be unremovable.
  it('keeps the inputs for a non-crew tag that already holds a link', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EditTagFormFields
        key={6}
        tag={makeTagDetail({
          id: 6,
          name: 'shoegaze',
          category: 'genre',
          social: { website: 'https://zine.test', instagram: null, bandcamp: null },
        })}
        onSuccess={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(screen.getByLabelText('Website')).toHaveValue('https://zine.test')
    await user.clear(screen.getByLabelText('Website'))
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    const [payload] = mockUpdateTagMutate.mock.calls[0]
    expect(payload.data.website).toBe('')
  })
})
