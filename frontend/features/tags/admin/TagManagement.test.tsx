import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { TagListItem } from '../types'

const mockUseTags = vi.fn()
vi.mock('../hooks', () => ({
  useTags: (...args: unknown[]) => mockUseTags(...args),
  useTag: vi.fn(),
  // MergeTagDialog (mounted inside TagManagement) pulls these in even when
  // the merge dialog is closed, so they have to exist in the mock.
  useSearchTags: () => ({ data: { tags: [] }, isLoading: false }),
  useTagAliases: () => ({ data: { aliases: [] }, isLoading: false }),
}))

vi.mock('./useAdminTags', () => ({
  useCreateTag: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateTag: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteTag: () => ({ mutate: vi.fn(), isPending: false }),
  useTagAliases: () => ({ data: { aliases: [] }, isLoading: false }),
  useCreateAlias: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteAlias: () => ({ mutate: vi.fn(), isPending: false }),
  useAllTagAliases: () => ({ data: { aliases: [], total: 0 }, isLoading: false, error: null }),
  useBulkImportAliases: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTags: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTagsPreview: () => ({ data: null, isLoading: false, error: null }),
  useLowQualityTagQueue: () => ({ data: { tags: [], total: 0 }, isLoading: false, error: null }),
  useSnoozeTag: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
  useMarkTagOfficial: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
  useGenreHierarchy: () => ({ data: { tags: [] }, isLoading: false, error: null }),
  useSetTagParent: () => ({ mutate: vi.fn(), isPending: false }),
}))

import { TagManagement } from './TagManagement'

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
