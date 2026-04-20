import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

// PSY-460 introduces a mobile "Show all tags" Sheet alongside the existing
// desktop top-5 cap. Both rows render in the DOM (swapped via Tailwind's
// `sm:hidden` / `hidden sm:flex` utilities — jsdom does not apply those
// styles). Existing assertions about "the visible pill row" predate that
// split and now match elements in both rows. The helper below scopes queries
// to the desktop row so the original behavior keeps being exercised; mobile-
// specific assertions go through the `entity-tag-list-mobile-row` /
// `entity-tag-list-mobile-sheet` testids instead.
function desktopRow() {
  return screen.getByTestId('entity-tag-list-desktop-row')
}

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// Shape mirrors EntityTagsResponse from features/tags/types.ts. Spelled out
// locally (rather than importing the type) so test fixtures keep working if
// the module under test is re-exported differently.
type MockEntityTag = {
  tag_id: number
  name: string
  slug: string
  category: string
  is_official: boolean
  upvotes: number
  downvotes: number
  wilson_score: number
  user_vote: number
  added_by_username?: string
  added_at?: string
}
type MockEntityTags = { tags: MockEntityTag[] }

const mockEntityTags: MockEntityTags = {
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
let currentMockTags: MockEntityTags = mockEntityTags
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
    const row = desktopRow()
    // The official tag "rock" should have a title tooltip indicating official status
    const rockLink = within(row).getByRole('link', { name: 'rock' })
    expect(rockLink).toHaveAttribute('title', 'rock (Official)')

    // The community tag "indie" should have a plain title
    const indieLink = within(row).getByRole('link', { name: 'indie' })
    expect(indieLink).toHaveAttribute('title', 'indie')

    // The visible BadgeCheck icon marker is present exactly once within the
    // desktop row (only on the official tag) so the distinction is not
    // tooltip-only.
    const officialMarkers = within(row).getAllByRole('img', { name: 'Official tag' })
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

    // 7 tags total, only 5 should be visible in the desktop row.
    const tagLinks = within(desktopRow()).getAllByRole('link')
    expect(tagLinks).toHaveLength(5)
  })

  it('sorts tags by Wilson score (highest first)', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const tagLinks = within(desktopRow()).getAllByRole('link')
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

    expect(within(desktopRow()).getByText('Show 2 more')).toBeInTheDocument()
  })

  it('expands to show all tags when "Show N more" is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    await user.click(within(desktopRow()).getByText('Show 2 more'))

    // All 7 tags should now be visible in the desktop row.
    const tagLinks = within(desktopRow()).getAllByRole('link')
    expect(tagLinks).toHaveLength(7)
  })

  it('collapses back to 5 tags when "Show less" is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const row = desktopRow()
    // Expand
    await user.click(within(row).getByText('Show 2 more'))
    expect(within(row).getAllByRole('link')).toHaveLength(7)

    // Collapse
    await user.click(within(row).getByText('Show less'))
    expect(within(row).getAllByRole('link')).toHaveLength(5)
  })

  it('does not show expand button when 5 or fewer tags exist', () => {
    currentMockTags = mockEntityTags // only 2 tags
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    // Only the desktop cap chip is tested here — the mobile "Show all"
    // chip has its own suite below.
    expect(within(desktopRow()).queryByText(/Show \d+ more/)).not.toBeInTheDocument()
    expect(within(desktopRow()).queryByText('Show less')).not.toBeInTheDocument()
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

// PSY-443 / PSY-483: new_user tier cannot create new tags server-side
// (backend returns 403 CodeTagCreationForbidden). Mirror that gate client-side.
//
// PSY-443 originally rendered a disabled Create button + Radix tooltip, but
// dogfood (ISSUE-006, April 2026) found the tooltip was effectively invisible
// — touch users never see hover, and a casual mouse user wouldn't hover a
// button that already looks dead. PSY-483 replaces the disabled button with
// inline explanatory prose that's always visible, and removes the
// silently-disabled affordance entirely.
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

  it('hides the Create button for new_user tier and shows an always-visible tier-gate explanation linking to /help/tiers', async () => {
    mockAuthUser = { user_tier: 'new_user' }
    await openDialogAndSearch('brand-new-tag')

    await waitFor(() => {
      expect(screen.getByText('No matching tags found.')).toBeInTheDocument()
    })

    // No Create affordance at all — neither enabled nor disabled. The old
    // PSY-443 disabled-button + tooltip wrapper are both gone.
    expect(
      screen.queryByRole('button', { name: /Create "brand-new-tag"/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('tag-create-disabled')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('tag-create-disabled-wrapper')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('tag-create-disabled-tooltip')
    ).not.toBeInTheDocument()

    // Inline prose surfaces the tier gate without requiring hover/touch.
    const tierGate = screen.getByTestId('tag-create-tier-gate')
    expect(tierGate).toBeInTheDocument()
    expect(tierGate).toHaveTextContent(/Contributor tier/i)

    const learnMore = tierGate.querySelector('a')
    expect(learnMore).not.toBeNull()
    expect(learnMore).toHaveAttribute('href', '/help/tiers')

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

  it('renders an enabled Create button for contributor tier and no tier-gate prose', async () => {
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
      screen.queryByTestId('tag-create-tier-gate')
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
    expect(
      screen.queryByTestId('tag-create-tier-gate')
    ).not.toBeInTheDocument()
  })

  it('appends a "Learn more" link to the 403 error message as defense-in-depth', async () => {
    // Even if the gate is somehow bypassed and the backend returns the 403,
    // the inline error should still link to the tier docs.
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

// PSY-441: tag pill hover card exposes creator attribution (username + when
// the tag was applied) and vote counts. The card is backed by Radix
// HoverCard; this suite drives it through the controlled click/keyboard
// toggle that composes on top of hover — pointer hover is well-covered by
// Radix's own tests, so we focus on the pieces we added (attribution body
// rendering, graceful skipping when backend data is missing, vote/link
// regressions).
describe('EntityTagList tag pill attribution hover card', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = {
      tags: [
        {
          tag_id: 10,
          name: 'post-punk',
          slug: 'post-punk',
          category: 'genre',
          is_official: false,
          upvotes: 3,
          downvotes: 1,
          wilson_score: 0.34,
          user_vote: 0,
          added_by_username: 'testuser2',
        },
        {
          tag_id: 11,
          name: 'noise',
          slug: 'noise',
          category: 'genre',
          is_official: false,
          upvotes: 0,
          downvotes: 0,
          wilson_score: 0,
          user_vote: 0,
          // added_by_username deliberately omitted to exercise the skip path
        },
      ],
    }
    currentMockSearchTags = defaultMockSearchTags
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
  })

  it('opens the attribution card on click and shows username, vote counts, and tag link', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    // Scope to the desktop row: the mobile row renders the same pills, but
    // only one trigger needs to be exercised to validate the HoverCard.
    const row = desktopRow()
    const trigger = within(row).getByRole('group', { name: /post-punk tag details/i })
    await user.click(trigger)

    const card = await screen.findByTestId('tag-attribution-card-10')
    expect(card).toBeInTheDocument()

    // Username link points to the user profile slug. The card is portalled
    // outside the row so we query at screen scope.
    const userLink = within(card).getByRole('link', { name: /@testuser2/ })
    expect(userLink).toHaveAttribute('href', '/users/testuser2')

    // Vote counts render with the correct singular/plural agreement.
    expect(card).toHaveTextContent(/3\s+upvotes/)
    // Use a negative lookahead instead of \b — jest-dom normalises whitespace
    // so a trailing "downvote" (singular) is immediately followed by the next
    // block's "View tag details", not a word boundary.
    expect(card).toHaveTextContent(/1\s+downvote(?!s)/)

    // The "View tag details" action links to the canonical tag detail page.
    const detailLink = within(card).getByRole('link', { name: /view tag details/i })
    expect(detailLink).toHaveAttribute('href', '/tags/post-punk')
  })

  it('opens the attribution card via keyboard (Enter on the focused pill wrapper)', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const trigger = within(desktopRow()).getByRole('group', { name: /post-punk tag details/i })
    trigger.focus()
    expect(trigger).toHaveFocus()

    await user.keyboard('{Enter}')

    const card = await screen.findByTestId('tag-attribution-card-10')
    expect(card).toBeInTheDocument()
    expect(card).toHaveTextContent('@testuser2')
  })

  it('omits the "Added by" line when the backend did not return a username', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    // Open the hover card for the "noise" tag (no added_by_username).
    const trigger = within(desktopRow()).getByRole('group', { name: /noise tag details/i })
    await user.click(trigger)

    const card = await screen.findByTestId('tag-attribution-card-11')
    expect(card).toBeInTheDocument()

    // No "Added by" copy AND no anonymous/undefined leak.
    expect(card).not.toHaveTextContent(/Added by/i)
    expect(card).not.toHaveTextContent(/undefined/i)

    // Vote counts + detail link still render — graceful degradation, not a
    // blank card.
    expect(card).toHaveTextContent(/0\s+upvotes/)
    expect(card).toHaveTextContent(/0\s+downvotes/)
    const detailLink = within(card).getByRole('link', { name: /view tag details/i })
    expect(detailLink).toHaveAttribute('href', '/tags/noise')
  })

  it('renders relative time alongside the username when added_at is present', async () => {
    const recent = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    currentMockTags = {
      tags: [
        {
          tag_id: 20,
          name: 'shoegaze',
          slug: 'shoegaze',
          category: 'genre',
          is_official: false,
          upvotes: 1,
          downvotes: 0,
          wilson_score: 0.2,
          user_vote: 0,
          added_by_username: 'testuser3',
          added_at: recent,
        },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    await user.click(
      within(desktopRow()).getByRole('group', { name: /shoegaze tag details/i })
    )

    const card = await screen.findByTestId('tag-attribution-card-20')
    // formatRelativeTime output for a timestamp ~5 minutes ago.
    expect(card).toHaveTextContent(/minutes? ago/i)
  })

  it('does not regress the inner tag link or vote buttons when the pill is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    const row = desktopRow()
    // The inline tag-name link still points to the canonical detail page.
    const tagLink = within(row).getByRole('link', { name: 'post-punk' })
    expect(tagLink).toHaveAttribute('href', '/tags/post-punk')

    // Vote buttons are still present and independently clickable (the hover
    // card wrapper guards against its own toggle when a button is clicked,
    // so the vote mutation still fires).
    const upvoteButton = within(row).getByRole('button', { name: /upvote post-punk/i })
    await user.click(upvoteButton)
    // We don't assert on mutate args here — useVoteOnTag is a mocked noop;
    // the guarantee is that clicking the vote button does not throw, does
    // not navigate, and doesn't blow up on the stopPropagation handler.
    expect(upvoteButton).toBeInTheDocument()
  })
})

// PSY-460: narrow viewports collapse the tag row to MOBILE_VISIBLE_COUNT
// pills + a "Show all tags" chip that opens a bottom-sliding Sheet. The
// sheet re-renders the full tag list and exposes the add-tag flow in its
// header. Desktop behavior (top-5 cap + inline "Show N more") is covered
// above and remains unchanged.
describe('EntityTagList mobile collapsible Sheet', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentMockTags = mockManyTags // 7 tags, above the mobile cap of 3
    currentMockSearchTags = defaultMockSearchTags
    mockAuthUser = { user_tier: 'contributor' }
    mockAddMutationError = null
  })

  it('renders only the first 3 pills in the mobile row when more than 3 tags exist', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const mobileRow = screen.getByTestId('entity-tag-list-mobile-row')
    const tagLinks = within(mobileRow).getAllByRole('link')
    expect(tagLinks).toHaveLength(3)
    // Wilson-score order is preserved: punk(0.62), post-punk(0.60), rock(0.56).
    expect(tagLinks[0]).toHaveTextContent('punk')
    expect(tagLinks[1]).toHaveTextContent('post-punk')
    expect(tagLinks[2]).toHaveTextContent('rock')
  })

  it('shows a "Show all tags" chip in the mobile row with the hidden count', () => {
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    const trigger = screen.getByTestId('entity-tag-list-mobile-show-all')
    // 7 total - 3 visible = 4 in the drawer.
    expect(trigger).toHaveTextContent(/4 more/)
  })

  it('omits the "Show all" chip when 3 or fewer tags exist', () => {
    currentMockTags = mockEntityTags // 2 tags
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(
      screen.queryByTestId('entity-tag-list-mobile-show-all')
    ).not.toBeInTheDocument()
  })

  it('opens the Sheet with every tag and the "Add" action in its header', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByTestId('entity-tag-list-mobile-show-all'))

    const sheet = await screen.findByTestId('entity-tag-list-mobile-sheet')
    expect(sheet).toBeInTheDocument()
    // Title surfaces the total count up-front so users know the drawer
    // contains more than the mobile row exposed.
    expect(sheet).toHaveTextContent(/All tags \(7\)/)

    // All 7 tag pills are reachable inside the sheet.
    const tagLinks = within(sheet).getAllByRole('link')
    expect(tagLinks).toHaveLength(7)

    // The Add-tag action is present in the sheet header — required so mobile
    // users can still add tags without closing the drawer first.
    const sheetAdd = within(sheet).getByTestId('entity-tag-list-sheet-add')
    expect(sheetAdd).toBeInTheDocument()
  })

  it('renders the vote buttons inside the Sheet when authenticated', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByTestId('entity-tag-list-mobile-show-all'))

    const sheet = await screen.findByTestId('entity-tag-list-mobile-sheet')
    // Each of the 7 pills has upvote + downvote buttons, for a total of 14.
    const upvotes = within(sheet).getAllByRole('button', { name: /upvote /i })
    const downvotes = within(sheet).getAllByRole('button', { name: /downvote /i })
    expect(upvotes).toHaveLength(7)
    expect(downvotes).toHaveLength(7)
  })

  it('opens the Add-tag dialog when the sheet-header Add action is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByTestId('entity-tag-list-mobile-show-all'))
    const sheet = await screen.findByTestId('entity-tag-list-mobile-sheet')
    await user.click(within(sheet).getByTestId('entity-tag-list-sheet-add'))

    // The Add Tag dialog opens (closing the Sheet first so Radix portals
    // do not stack). We match on the dialog title because other tests
    // already cover the full dialog a11y surface.
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })
    expect(screen.getByText('Add Tag')).toBeInTheDocument()
  })

  it('preserves the official indicator inside the Sheet', async () => {
    const user = userEvent.setup()
    // Mix one official tag into the ordered list so the indicator assertion
    // is meaningful (mockManyTags has all is_official=false by default).
    currentMockTags = {
      tags: [
        ...mockManyTags.tags,
        {
          tag_id: 100,
          name: 'official-tag',
          slug: 'official-tag',
          category: 'genre',
          is_official: true,
          upvotes: 10,
          downvotes: 0,
          wilson_score: 0.91,
          user_vote: 0,
        },
      ],
    }

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    await user.click(screen.getByTestId('entity-tag-list-mobile-show-all'))
    const sheet = await screen.findByTestId('entity-tag-list-mobile-sheet')

    const officialMarkers = within(sheet).getAllByRole('img', { name: 'Official tag' })
    expect(officialMarkers).toHaveLength(1)
    expect(within(sheet).getByRole('link', { name: 'official-tag' })).toHaveAttribute(
      'title',
      'official-tag (Official)'
    )
  })

  it('opens the attribution hover card inside the Sheet (Radix portal compatibility)', async () => {
    // Keep one tag with full attribution data so the hover card body has
    // something to render.
    currentMockTags = {
      tags: [
        {
          tag_id: 30,
          name: 'post-punk',
          slug: 'post-punk',
          category: 'genre',
          is_official: false,
          upvotes: 3,
          downvotes: 1,
          wilson_score: 0.34,
          user_vote: 0,
          added_by_username: 'testuser',
        },
        // Pad with 3 more tags so the "Show all" chip still renders.
        { tag_id: 31, name: 'a', slug: 'a', category: 'genre', is_official: false, upvotes: 0, downvotes: 0, wilson_score: 0.1, user_vote: 0 },
        { tag_id: 32, name: 'b', slug: 'b', category: 'genre', is_official: false, upvotes: 0, downvotes: 0, wilson_score: 0.1, user_vote: 0 },
        { tag_id: 33, name: 'c', slug: 'c', category: 'genre', is_official: false, upvotes: 0, downvotes: 0, wilson_score: 0.1, user_vote: 0 },
      ],
    }

    const user = userEvent.setup()
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    await user.click(screen.getByTestId('entity-tag-list-mobile-show-all'))
    const sheet = await screen.findByTestId('entity-tag-list-mobile-sheet')

    // Click the pill inside the sheet — Radix HoverCardContent portals to
    // document.body, not into the sheet container, so we query the card at
    // screen scope. If Radix portals fought with the Sheet's own portal we'd
    // either not find the card or get an overlap conflict; either shows up
    // here as a failure.
    await user.click(
      within(sheet).getByRole('group', { name: /post-punk tag details/i })
    )

    const card = await screen.findByTestId('tag-attribution-card-30')
    expect(card).toBeInTheDocument()
    expect(card).toHaveTextContent('@testuser')
  })
})
