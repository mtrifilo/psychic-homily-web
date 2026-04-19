import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const mockEntityTags = {
  tags: [
    { tag_id: 1, name: 'rock', slug: 'rock', category: 'genre', is_official: true, upvotes: 3, downvotes: 0, wilson_score: 0.56, user_vote: 0 },
    { tag_id: 2, name: 'indie', slug: 'indie', category: 'genre', is_official: false, upvotes: 1, downvotes: 0, wilson_score: 0.21, user_vote: 0 },
  ],
}

const mockManyTags = {
  tags: [
    { tag_id: 1, name: 'rock', slug: 'rock', category: 'genre', is_official: false, upvotes: 3, downvotes: 0, wilson_score: 0.56, user_vote: 0 },
    { tag_id: 2, name: 'indie', slug: 'indie', category: 'genre', is_official: false, upvotes: 1, downvotes: 0, wilson_score: 0.21, user_vote: 0 },
    { tag_id: 3, name: 'punk', slug: 'punk', category: 'genre', is_official: false, upvotes: 5, downvotes: 1, wilson_score: 0.62, user_vote: 0 },
    { tag_id: 4, name: 'shoegaze', slug: 'shoegaze', category: 'genre', is_official: false, upvotes: 2, downvotes: 0, wilson_score: 0.34, user_vote: 0 },
    { tag_id: 5, name: 'post-punk', slug: 'post-punk', category: 'genre', is_official: false, upvotes: 4, downvotes: 0, wilson_score: 0.60, user_vote: 0 },
    { tag_id: 6, name: 'noise', slug: 'noise', category: 'genre', is_official: false, upvotes: 0, downvotes: 0, wilson_score: 0.0, user_vote: 0 },
    { tag_id: 7, name: 'experimental', slug: 'experimental', category: 'genre', is_official: false, upvotes: 1, downvotes: 1, wilson_score: 0.09, user_vote: 0 },
  ],
}

const defaultMockSearchTags = {
  tags: [
    { id: 3, name: 'punk', slug: 'punk', category: 'genre', is_official: false, usage_count: 5, created_at: '' },
  ],
}

const mockAddMutate = vi.fn()
let currentMockTags = mockEntityTags
let currentMockSearchTags: typeof defaultMockSearchTags = defaultMockSearchTags

vi.mock('../hooks', () => ({
  useEntityTags: () => ({
    data: currentMockTags,
    isLoading: false,
  }),
  useAddTagToEntity: () => ({
    mutate: mockAddMutate,
    isPending: false,
    error: null,
  }),
  useRemoveTagFromEntity: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useVoteOnTag: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useRemoveTagVote: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useSearchTags: () => ({
    data: currentMockSearchTags,
    isLoading: false,
  }),
}))

vi.mock('../types', () => ({
  getCategoryColor: () => '',
  getCategoryLabel: (cat: string) => cat.charAt(0).toUpperCase() + cat.slice(1),
  TAG_CATEGORIES: ['genre', 'locale', 'other'],
}))

import { EntityTagList } from './EntityTagList'

describe('EntityTagList add-tag dialog accessibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockEntityTags
    currentMockSearchTags = defaultMockSearchTags
  })

  it('renders the Add button when authenticated', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )
    expect(screen.getByRole('button', { name: 'Add tag' })).toBeInTheDocument()
  })

  it('does not render the Add button when not authenticated', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )
    expect(screen.queryByRole('button', { name: 'Add tag' })).not.toBeInTheDocument()
  })

  it('shows official badge icon for official tags and not for community tags', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )
    // The official tag "rock" should have a title tooltip indicating official status
    const rockLink = screen.getByRole('link', { name: 'rock' })
    expect(rockLink).toHaveAttribute('title', 'rock (Official)')

    // The community tag "indie" should have a plain title
    const indieLink = screen.getByRole('link', { name: 'indie' })
    expect(indieLink).toHaveAttribute('title', 'indie')
  })

  it('opens add-tag dialog with title and no aria-describedby attribute', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByRole('button', { name: 'Add tag' }))

    // Dialog should be open with a title
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })
    expect(screen.getByText('Add Tag')).toBeInTheDocument()

    // The dialog should NOT have aria-describedby (we passed undefined to suppress it)
    const dialog = screen.getByRole('dialog')
    expect(dialog).not.toHaveAttribute('aria-describedby')
  })

  it('submits first search result when Enter is pressed with matching tags', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    // Open the dialog
    await user.click(screen.getByRole('button', { name: 'Add tag' }))
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })

    // Type a search query (>= 2 chars to trigger search)
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, 'punk')

    // Wait for search results to appear
    await waitFor(() => {
      expect(screen.getByText('punk')).toBeInTheDocument()
    })

    // Press Enter
    await user.keyboard('{Enter}')

    // Should have called the add mutation with the first result (tag id 3)
    expect(mockAddMutate).toHaveBeenCalledWith(
      expect.objectContaining({ entityType: 'artist', entityId: 1, tag_id: 3 }),
      expect.any(Object)
    )
  })
})

describe('EntityTagList top-5 cap and Wilson score sorting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockManyTags
    currentMockSearchTags = defaultMockSearchTags
  })

  it('shows only top 5 tags by default when more than 5 exist', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    // 7 tags total, only 5 should be visible
    const tagLinks = screen.getAllByRole('link')
    expect(tagLinks).toHaveLength(5)
  })

  it('sorts tags by Wilson score (highest first)', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const tagLinks = screen.getAllByRole('link')
    // Expected order by wilson_score descending: punk(0.62), post-punk(0.60), rock(0.56), shoegaze(0.34), indie(0.21)
    expect(tagLinks[0]).toHaveTextContent('punk')
    expect(tagLinks[1]).toHaveTextContent('post-punk')
    expect(tagLinks[2]).toHaveTextContent('rock')
    expect(tagLinks[3]).toHaveTextContent('shoegaze')
    expect(tagLinks[4]).toHaveTextContent('indie')
  })

  it('shows "Show N more" button when tags exceed the cap', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(screen.getByText('Show 2 more')).toBeInTheDocument()
  })

  it('expands to show all tags when "Show N more" is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    await user.click(screen.getByText('Show 2 more'))

    // All 7 tags should now be visible
    const tagLinks = screen.getAllByRole('link')
    expect(tagLinks).toHaveLength(7)
  })

  it('collapses back to 5 tags when "Show less" is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    // Expand
    await user.click(screen.getByText('Show 2 more'))
    expect(screen.getAllByRole('link')).toHaveLength(7)

    // Collapse
    await user.click(screen.getByText('Show less'))
    expect(screen.getAllByRole('link')).toHaveLength(5)
  })

  it('does not show expand button when 5 or fewer tags exist', () => {
    currentMockTags = mockEntityTags // only 2 tags
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(screen.queryByText(/Show \d+ more/)).not.toBeInTheDocument()
    expect(screen.queryByText('Show less')).not.toBeInTheDocument()
  })
})

// PSY-442: alias transparency in the add-tag autocomplete.
// When the backend indicates an autocomplete row matched via `tag_aliases`
// rather than `tags.name`, the dialog must render a small caption under
// the tag name so the user sees which term was interpreted as the
// canonical form. Rows that matched by name render unchanged.
describe('EntityTagList add-tag dialog alias caption', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockEntityTags
    currentMockSearchTags = defaultMockSearchTags
  })

  async function openDialogAndSearch(queryText: string) {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )
    await user.click(screen.getByRole('button', { name: 'Add tag' }))
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, queryText)
    return user
  }

  it('renders the "matched" caption when a result carries matched_via_alias', async () => {
    currentMockSearchTags = {
      tags: [
        {
          id: 3,
          name: 'punk',
          slug: 'punk',
          category: 'genre',
          is_official: false,
          usage_count: 15,
          created_at: '',
          matched_via_alias: 'punk-rock',
        },
      ],
    }

    await openDialogAndSearch('punk-rock')

    await waitFor(() => {
      expect(screen.getByText('punk')).toBeInTheDocument()
    })

    const caption = screen.getByTestId('tag-autocomplete-matched-alias')
    expect(caption).toBeInTheDocument()
    expect(caption).toHaveTextContent(/matched\s+[“"]punk-rock[”"]/)
  })

  it('omits the caption for rows matched by name', async () => {
    // The default search mock does NOT set matched_via_alias — that
    // mirrors the "user typed the canonical form" case.
    await openDialogAndSearch('punk')

    await waitFor(() => {
      expect(screen.getByText('punk')).toBeInTheDocument()
    })

    expect(
      screen.queryByTestId('tag-autocomplete-matched-alias')
    ).not.toBeInTheDocument()
  })

  it('renders captions only on the rows that matched via alias in a mixed result set', async () => {
    currentMockSearchTags = {
      tags: [
        {
          id: 3,
          name: 'punk',
          slug: 'punk',
          category: 'genre',
          is_official: false,
          usage_count: 15,
          created_at: '',
          matched_via_alias: 'punk-rock',
        },
        {
          id: 4,
          name: 'post-punk',
          slug: 'post-punk',
          category: 'genre',
          is_official: false,
          usage_count: 7,
          created_at: '',
          // no matched_via_alias — matched by name
        },
      ],
    }

    await openDialogAndSearch('punk')

    await waitFor(() => {
      expect(screen.getByText('punk')).toBeInTheDocument()
      expect(screen.getByText('post-punk')).toBeInTheDocument()
    })

    // Exactly one row has a caption — the one whose match came through the
    // alias table.
    const captions = screen.getAllByTestId('tag-autocomplete-matched-alias')
    expect(captions).toHaveLength(1)
    expect(captions[0]).toHaveTextContent(/matched\s+[“"]punk-rock[”"]/)
  })
})
