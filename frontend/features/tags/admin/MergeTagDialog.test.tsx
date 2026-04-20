import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { MergeTagsPreview, TagListItem } from '../types'

const mockUseSearchTags = vi.fn()
vi.mock('../hooks', () => ({
  useSearchTags: (...args: unknown[]) => mockUseSearchTags(...args),
}))

const mockMutate = vi.fn()
const mockUsePreview = vi.fn()
const mockUseTagAliases = vi.fn()
vi.mock('./useAdminTags', () => ({
  useMergeTags: () => ({ mutate: mockMutate, isPending: false }),
  useMergeTagsPreview: (...args: unknown[]) => mockUsePreview(...args),
  useTagAliases: (...args: unknown[]) => mockUseTagAliases(...args),
}))

import { MergeTagDialog } from './MergeTagDialog'

function makeTag(overrides: Partial<TagListItem> = {}): TagListItem {
  return {
    id: 2,
    name: 'shoegaze',
    slug: 'shoegaze',
    category: 'genre',
    is_official: false,
    usage_count: 10,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

function makePreview(overrides: Partial<MergeTagsPreview> = {}): MergeTagsPreview {
  return {
    moved_entity_tags: 5,
    moved_votes: 3,
    moved_upvotes: 2,
    moved_downvotes: 1,
    skipped_entity_tags: 0,
    skipped_votes: 0,
    source_aliases_count: 0,
    source_name: 'shoe-gaze',
    target_name: 'shoegaze',
    ...overrides,
  }
}

describe('MergeTagDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTagAliases.mockReturnValue({ data: { aliases: [] } })
    mockUseSearchTags.mockReturnValue({ data: { tags: [] }, isLoading: false })
    mockUsePreview.mockReturnValue({ data: null, isLoading: false, error: null })
  })

  it('shows the source name in the dialog title', () => {
    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )
    expect(screen.getByText(/Merge "shoe-gaze" into/)).toBeInTheDocument()
  })

  it('searches when 2+ characters typed and filters out source tag', async () => {
    mockUseSearchTags.mockImplementation((query: string) => {
      if (query === 'shoe') {
        return {
          data: {
            tags: [
              makeTag({ id: 1, name: 'shoe-gaze' }), // this IS the source
              makeTag({ id: 2, name: 'shoegaze' }),
            ],
          },
          isLoading: false,
        }
      }
      return { data: { tags: [] }, isLoading: false }
    })

    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )

    const input = screen.getByPlaceholderText(/Type at least 2 characters/)
    fireEvent.change(input, { target: { value: 'shoe' } })

    await waitFor(() => {
      // Only the non-source candidate is listed.
      expect(screen.getByText('shoegaze')).toBeInTheDocument()
    })
    // Source is hidden.
    expect(screen.queryByRole('button', { name: /shoe-gaze/i })).toBeNull()
  })

  it('renders preview counts after selecting a target', async () => {
    mockUseSearchTags.mockReturnValue({
      data: { tags: [makeTag({ id: 2, name: 'shoegaze' })] },
      isLoading: false,
    })
    mockUsePreview.mockReturnValue({
      data: makePreview({
        moved_entity_tags: 5,
        moved_votes: 3,
        moved_upvotes: 2,
        moved_downvotes: 1,
        skipped_entity_tags: 1,
        source_aliases_count: 2,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )

    const input = screen.getByPlaceholderText(/Type at least 2 characters/)
    fireEvent.change(input, { target: { value: 'shoe' } })
    await waitFor(() => {
      expect(screen.getByText('shoegaze')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('shoegaze'))

    const preview = await screen.findByTestId('merge-preview')
    // PSY-487 summary line uses an explicit single-sentence format that
    // breaks votes into upvotes/downvotes and surfaces alias count.
    const summary = await screen.findByTestId('merge-preview-summary')
    expect(summary).toHaveTextContent('5 entity applications')
    expect(summary).toHaveTextContent('2 upvotes')
    expect(summary).toHaveTextContent('1 downvote')
    expect(summary).toHaveTextContent('2 aliases')
    expect(summary).toHaveTextContent(/into "shoegaze"/)
    // Detail lines stay around for deeper context.
    expect(preview).toHaveTextContent(/1 duplicate entity tag/)
  })

  it('summary line uses singular "alias" when source has exactly one alias', async () => {
    mockUseSearchTags.mockReturnValue({
      data: { tags: [makeTag({ id: 2, name: 'shoegaze' })] },
      isLoading: false,
    })
    mockUsePreview.mockReturnValue({
      data: makePreview({
        moved_entity_tags: 1,
        moved_votes: 2,
        moved_upvotes: 2,
        moved_downvotes: 0,
        source_aliases_count: 0,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )

    fireEvent.change(screen.getByPlaceholderText(/Type at least 2 characters/), {
      target: { value: 'shoe' },
    })
    await waitFor(() => expect(screen.getByText('shoegaze')).toBeInTheDocument())
    fireEvent.click(screen.getByText('shoegaze'))

    const summary = await screen.findByTestId('merge-preview-summary')
    expect(summary).toHaveTextContent('1 entity application')
    expect(summary).toHaveTextContent('2 upvotes')
    expect(summary).toHaveTextContent('0 downvotes')
    expect(summary).toHaveTextContent('0 aliases')
  })

  it('calls merge mutation on confirm', async () => {
    mockUseSearchTags.mockReturnValue({
      data: { tags: [makeTag({ id: 2, name: 'shoegaze' })] },
      isLoading: false,
    })
    mockUsePreview.mockReturnValue({
      data: makePreview(),
      isLoading: false,
      error: null,
    })

    const onClose = vi.fn()
    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={onClose}
      />
    )

    fireEvent.change(screen.getByPlaceholderText(/Type at least 2 characters/), {
      target: { value: 'shoe' },
    })
    await waitFor(() => expect(screen.getByText('shoegaze')).toBeInTheDocument())
    fireEvent.click(screen.getByText('shoegaze'))

    const confirm = await screen.findByRole('button', { name: /^Merge/ })
    fireEvent.click(confirm)

    expect(mockMutate).toHaveBeenCalledWith(
      { sourceId: 1, targetId: 2 },
      expect.any(Object)
    )
  })

  it('surfaces preview errors instead of allowing confirm', async () => {
    mockUseSearchTags.mockReturnValue({
      data: { tags: [makeTag({ id: 2, name: 'shoegaze' })] },
      isLoading: false,
    })
    mockUsePreview.mockReturnValue({
      data: null,
      isLoading: false,
      error: new Error('alias conflict'),
    })

    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )

    fireEvent.change(screen.getByPlaceholderText(/Type at least 2 characters/), {
      target: { value: 'shoe' },
    })
    await waitFor(() => expect(screen.getByText('shoegaze')).toBeInTheDocument())
    fireEvent.click(screen.getByText('shoegaze'))

    expect(await screen.findByText(/alias conflict/)).toBeInTheDocument()
    const confirm = screen.getByRole('button', { name: /^Merge/ })
    expect(confirm).toBeDisabled()
  })

  it('hides source aliases from target candidates', async () => {
    mockUseTagAliases.mockReturnValue({
      data: { aliases: [{ id: 999, alias: 'shoe-gaze', created_at: '' }] },
    })
    // For this test, the hook returns alias IDs as number[] inside the
    // component's useSourceAliasIds. The dialog maps `alias.id`, so the
    // alias row id (999) would be excluded from the candidates list.
    mockUseSearchTags.mockReturnValue({
      data: {
        tags: [
          makeTag({ id: 2, name: 'shoegaze' }),
          makeTag({ id: 999, name: 'shoe-gaze-alias-entry' }),
        ],
      },
      isLoading: false,
    })

    renderWithProviders(
      <MergeTagDialog
        open
        sourceTagId={1}
        sourceTagName="shoe-gaze"
        onClose={vi.fn()}
      />
    )

    fireEvent.change(screen.getByPlaceholderText(/Type at least 2 characters/), {
      target: { value: 'shoe' },
    })
    await waitFor(() => expect(screen.getByText('shoegaze')).toBeInTheDocument())
    expect(screen.queryByText('shoe-gaze-alias-entry')).toBeNull()
  })
})
