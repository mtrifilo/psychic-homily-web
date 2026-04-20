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

type MockSearchTag = {
  id: number
  name: string
  slug: string
  category: string
  is_official: boolean
  usage_count: number
  created_at: string
  matched_via_alias?: string
}
type MockSearchTags = { tags: MockSearchTag[] }

const defaultMockSearchTags: MockSearchTags = {
  tags: [
    { id: 3, name: 'punk', slug: 'punk', category: 'genre', is_official: false, usage_count: 5, created_at: '' },
  ],
}

const mockAddMutate = vi.fn()
let currentMockTags = mockEntityTags
let currentMockSearchTags: MockSearchTags = defaultMockSearchTags
let mockAddMutationError: Error | null = null

vi.mock('../hooks', () => ({
  useEntityTags: () => ({
    data: currentMockTags,
    isLoading: false,
  }),
  useAddTagToEntity: () => ({
    mutate: mockAddMutate,
    isPending: false,
    error: mockAddMutationError,
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

// Default auth context: a contributor (can create tags). Individual tests
// override `mockAuthUser` to exercise the new_user disabled-Create path
// added in PSY-443.
type MockAuthUser = { user_tier?: string } | null
let mockAuthUser: MockAuthUser = { user_tier: 'contributor' }
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: mockAuthUser,
    isAuthenticated: Boolean(mockAuthUser),
    isLoading: false,
    error: null,
    setUser: vi.fn(),
    setError: vi.fn(),
    clearError: vi.fn(),
    logout: vi.fn(),
  }),
}))

import { EntityTagList } from './EntityTagList'

describe('EntityTagList add-tag dialog accessibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockEntityTags
    currentMockSearchTags = defaultMockSearchTags
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
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

    // The visible BadgeCheck icon marker is present exactly once (only on
    // the official tag) so the distinction is not tooltip-only.
    const officialMarkers = screen.getAllByRole('img', { name: 'Official tag' })
    expect(officialMarkers).toHaveLength(1)

    // And the official pill wrapper carries the primary-accent background
    // so it reads as curated at a glance (ISSUE-004 tags-audit-2).
    const officialPill = officialMarkers[0].closest('div')
    expect(officialPill?.className).toContain('bg-primary/10')
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
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
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
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
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

// PSY-452: when an alias resolves to a tag that's ALREADY applied to the
// current entity, the add-tag dialog must surface an "already applied" row
// and suppress the Create CTA. Previously the search-result filter would
// silently drop the canonical row, leaving the dialog with zero results and
// inviting the user to create a duplicate tag under the alias string.
describe('EntityTagList add-tag dialog already-applied short-circuit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockEntityTags
    currentMockSearchTags = defaultMockSearchTags
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
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

  it('shows "already applied" message and suppresses Create when alias resolves to an applied tag', async () => {
    // mockEntityTags already includes tag id 1 ("rock"). Pretend the user
    // typed "rock-music" and the backend returned the canonical "rock" row
    // via its alias — that row should be filtered out AND short-circuit the
    // Create CTA.
    currentMockSearchTags = {
      tags: [
        {
          id: 1,
          name: 'rock',
          slug: 'rock',
          category: 'genre',
          is_official: false,
          usage_count: 42,
          created_at: '',
          matched_via_alias: 'rock-music',
        },
      ],
    }

    await openDialogAndSearch('rock-music')

    await waitFor(() => {
      expect(
        screen.getByTestId('tag-autocomplete-already-applied')
      ).toBeInTheDocument()
    })

    const row = screen.getByTestId('tag-autocomplete-already-applied')
    expect(row).toHaveTextContent(/[“"]rock[”"] is already applied/)

    // PSY-442 alias caption still renders — the transparency story is
    // preserved for the already-applied edge case too.
    const caption = screen.getByTestId('tag-autocomplete-matched-alias')
    expect(caption).toHaveTextContent(/matched\s+[“"]rock-music[”"]/)

    // Create CTA must be suppressed.
    expect(
      screen.queryByRole('button', { name: /Create "rock-music"/ })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('No matching tags found.')).not.toBeInTheDocument()
  })

  it('shows "already applied" even when the match is by canonical name (no alias)', async () => {
    // User typed the canonical name of an already-applied tag — backend
    // returns the row with no matched_via_alias, and the same filter removes
    // it. Still should short-circuit to "already applied" instead of Create.
    currentMockSearchTags = {
      tags: [
        {
          id: 2,
          name: 'indie',
          slug: 'indie',
          category: 'genre',
          is_official: false,
          usage_count: 8,
          created_at: '',
        },
      ],
    }

    await openDialogAndSearch('indie')

    await waitFor(() => {
      expect(
        screen.getByTestId('tag-autocomplete-already-applied')
      ).toBeInTheDocument()
    })

    expect(
      screen.getByTestId('tag-autocomplete-already-applied')
    ).toHaveTextContent(/[“"]indie[”"] is already applied/)

    // No alias caption when the match was by name.
    expect(
      screen.queryByTestId('tag-autocomplete-matched-alias')
    ).not.toBeInTheDocument()

    // Create CTA must be suppressed.
    expect(
      screen.queryByRole('button', { name: /Create "indie"/ })
    ).not.toBeInTheDocument()
  })

  it('still offers Create when the query truly matches nothing that exists', async () => {
    // Empty search result — no applied tag matches, so the original "No
    // matching tags found" + Create CTA flow still applies.
    currentMockSearchTags = { tags: [] }

    await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    expect(
      screen.queryByTestId('tag-autocomplete-already-applied')
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Create "brand-new-tag"/ })
    ).toBeInTheDocument()
  })

  it('does not short-circuit Enter into a Create when an alias matches an applied tag', async () => {
    currentMockSearchTags = {
      tags: [
        {
          id: 1,
          name: 'rock',
          slug: 'rock',
          category: 'genre',
          is_official: false,
          usage_count: 42,
          created_at: '',
          matched_via_alias: 'rock-music',
        },
      ],
    }

    const user = await openDialogAndSearch('rock-music')

    await waitFor(() => {
      expect(
        screen.getByTestId('tag-autocomplete-already-applied')
      ).toBeInTheDocument()
    })

    await user.keyboard('{Enter}')

    // The add mutation must not be called — neither as a select nor as a
    // create — because the tag is already applied.
    expect(mockAddMutate).not.toHaveBeenCalled()
  })
})

// PSY-443: new_user tier cannot create new tags server-side (backend returns
// 403 CodeTagCreationForbidden). Mirror that gate client-side with a disabled
// Create button + tooltip linking to /help/tiers, so users see guidance
// instead of a dead-end error after clicking.
describe('EntityTagList add-tag dialog create-tag tier gating', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockEntityTags
    currentMockSearchTags = { tags: [] }
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

  it('disables the Create button for new_user tier with a tooltip that links to /help/tiers', async () => {
    mockAuthUser = { user_tier: 'new_user' }
    const user = await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    const disabledButton = screen.getByTestId('tag-create-disabled')
    expect(disabledButton).toBeDisabled()
    expect(disabledButton).toHaveAttribute('aria-disabled', 'true')

    // Tooltip fires on hover of the wrapper (span lets a disabled button
    // still participate in a Radix tooltip).
    await user.hover(screen.getByTestId('tag-create-disabled-wrapper'))

    await waitFor(() => {
      const tooltip = screen.getByTestId('tag-create-disabled-tooltip')
      expect(tooltip).toBeInTheDocument()
      expect(tooltip).toHaveTextContent(/Reach Contributor tier/i)
      const learnMore = tooltip.querySelector('a')
      expect(learnMore).not.toBeNull()
      expect(learnMore).toHaveAttribute('href', '/help/tiers')
    })

    expect(mockAddMutate).not.toHaveBeenCalled()
  })

  it('does not trigger a create via Enter for new_user tier', async () => {
    mockAuthUser = { user_tier: 'new_user' }
    const user = await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    await user.keyboard('{Enter}')
    expect(mockAddMutate).not.toHaveBeenCalled()
  })

  it('renders an enabled Create button for contributor tier', async () => {
    mockAuthUser = { user_tier: 'contributor' }
    await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    const createButton = screen.getByRole('button', {
      name: /Create "brand-new-tag"/,
    })
    expect(createButton).not.toBeDisabled()
    expect(
      screen.queryByTestId('tag-create-disabled')
    ).not.toBeInTheDocument()
  })

  it('renders an enabled Create button for trusted_contributor tier', async () => {
    mockAuthUser = { user_tier: 'trusted_contributor' }
    await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    const createButton = screen.getByRole('button', {
      name: /Create "brand-new-tag"/,
    })
    expect(createButton).not.toBeDisabled()
  })

  it('appends a "Learn more" link to the 403 error message as defense-in-depth', async () => {
    // Even if the disabled-button gate is somehow bypassed and the backend
    // returns the 403, the inline error should still link to the tier docs.
    mockAuthUser = { user_tier: 'new_user' }
    mockAddMutationError = new Error(
      'New users can only apply existing tags. Reach Contributor tier to create new tags.'
    )

    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )
    await user.click(screen.getByRole('button', { name: 'Add tag' }))

    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })

    const errorText = screen.getByText(
      /New users can only apply existing tags/i
    )
    expect(errorText).toBeInTheDocument()

    const learnMore = errorText.querySelector('a')
    expect(learnMore).not.toBeNull()
    expect(learnMore).toHaveAttribute('href', '/help/tiers')
  })
})
