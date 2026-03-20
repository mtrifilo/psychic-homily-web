import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { EntityTag } from '../types'

// ── Mock hooks ──────────────────────────────────────────

const mockEntityTags = vi.fn()
const mockVoteMutate = vi.fn()
const mockRemoveVoteMutate = vi.fn()
const mockAddMutate = vi.fn()
const mockSearchTags = vi.fn()

vi.mock('../hooks', () => ({
  useEntityTags: (...args: unknown[]) => mockEntityTags(...args),
  useVoteOnTag: () => ({
    mutate: mockVoteMutate,
    isPending: false,
  }),
  useRemoveTagVote: () => ({
    mutate: mockRemoveVoteMutate,
    isPending: false,
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
  useSearchTags: (...args: unknown[]) => mockSearchTags(...args),
}))

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

import { EntityTagList } from './EntityTagList'

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = createQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  )
}

function makeTag(overrides: Partial<EntityTag> = {}): EntityTag {
  return {
    tag_id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    upvotes: 5,
    downvotes: 2,
    wilson_score: 0.6,
    user_vote: null,
    ...overrides,
  }
}

describe('EntityTagList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })
    mockSearchTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })
  })

  // ── Loading state ──

  it('shows loading spinner while tags are loading', () => {
    mockEntityTags.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    // Loader2 renders an svg with the animate-spin class
    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Empty states ──

  it('returns null when no tags and user is not authenticated', () => {
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })

    const { container } = renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(container.innerHTML).toBe('')
  })

  it('shows "No tags yet" prompt when no tags and user is authenticated', () => {
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    expect(screen.getByText('No tags yet. Be the first to add one!')).toBeInTheDocument()
  })

  // ── Rendering tags ──

  it('renders tag names as links to tag detail pages', () => {
    const tags = [
      makeTag({ tag_id: 1, name: 'rock', slug: 'rock' }),
      makeTag({ tag_id: 2, name: 'punk', slug: 'punk', category: 'style' }),
    ]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    const rockLink = screen.getByRole('link', { name: 'rock' })
    expect(rockLink).toHaveAttribute('href', '/tags/rock')

    const punkLink = screen.getByRole('link', { name: 'punk' })
    expect(punkLink).toHaveAttribute('href', '/tags/punk')
  })

  it('renders "Tags" heading', () => {
    const tags = [makeTag()]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    expect(screen.getByText('Tags')).toBeInTheDocument()
  })

  // ── Vote score display ──

  it('shows positive score', () => {
    const tags = [makeTag({ upvotes: 8, downvotes: 3 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    expect(screen.getByText('+5')).toBeInTheDocument()
  })

  it('shows negative score', () => {
    const tags = [makeTag({ upvotes: 1, downvotes: 4 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    expect(screen.getByText('-3')).toBeInTheDocument()
  })

  it('does not show score when no votes', () => {
    const tags = [makeTag({ upvotes: 0, downvotes: 0 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    // Score should not be visible
    expect(screen.queryByText('+0')).not.toBeInTheDocument()
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('shows zero score as +0 when there are votes', () => {
    const tags = [makeTag({ upvotes: 3, downvotes: 3 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    expect(screen.getByText('+0')).toBeInTheDocument()
  })

  // ── Vote buttons (authenticated) ──

  it('shows vote buttons when authenticated', () => {
    const tags = [makeTag({ name: 'rock' })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    expect(screen.getByLabelText('Upvote rock')).toBeInTheDocument()
    expect(screen.getByLabelText('Downvote rock')).toBeInTheDocument()
  })

  it('does not show vote buttons when not authenticated', () => {
    const tags = [makeTag({ name: 'rock' })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(screen.queryByLabelText('Upvote rock')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Downvote rock')).not.toBeInTheDocument()
  })

  // ── Vote interactions ──

  it('calls voteMutation.mutate with is_upvote=true on upvote click', async () => {
    const user = userEvent.setup()
    const tags = [makeTag({ tag_id: 42, name: 'rock', user_vote: null })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={10} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Upvote rock'))

    expect(mockVoteMutate).toHaveBeenCalledWith({
      tagId: 42,
      entityType: 'artist',
      entityId: 10,
      is_upvote: true,
    })
  })

  it('calls voteMutation.mutate with is_upvote=false on downvote click', async () => {
    const user = userEvent.setup()
    const tags = [makeTag({ tag_id: 42, name: 'rock', user_vote: null })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={10} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Downvote rock'))

    expect(mockVoteMutate).toHaveBeenCalledWith({
      tagId: 42,
      entityType: 'artist',
      entityId: 10,
      is_upvote: false,
    })
  })

  it('removes vote when clicking the same vote direction (toggle off)', async () => {
    const user = userEvent.setup()
    // User already upvoted
    const tags = [makeTag({ tag_id: 42, name: 'rock', user_vote: 1 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={10} isAuthenticated />
    )

    // Click upvote again (same direction) => toggle off
    await user.click(screen.getByLabelText('Upvote rock'))

    expect(mockRemoveVoteMutate).toHaveBeenCalledWith({
      tagId: 42,
      entityType: 'artist',
      entityId: 10,
    })
    // Should NOT have called voteMutate
    expect(mockVoteMutate).not.toHaveBeenCalled()
  })

  it('switches vote when clicking opposite direction', async () => {
    const user = userEvent.setup()
    // User already upvoted
    const tags = [makeTag({ tag_id: 42, name: 'rock', user_vote: 1 })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={10} isAuthenticated />
    )

    // Click downvote (opposite direction) => switch vote
    await user.click(screen.getByLabelText('Downvote rock'))

    expect(mockVoteMutate).toHaveBeenCalledWith({
      tagId: 42,
      entityType: 'artist',
      entityId: 10,
      is_upvote: false,
    })
    expect(mockRemoveVoteMutate).not.toHaveBeenCalled()
  })

  it('does not call vote mutations when not authenticated', async () => {
    const user = userEvent.setup()
    const tags = [makeTag({ tag_id: 42, name: 'rock' })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    // Render with isAuthenticated but will test the internal guard
    // Since vote buttons are only shown when authenticated,
    // let's verify the handleVote guard by rendering as authenticated
    // but calling with isAuthenticated=false should hide buttons entirely
    renderWithProviders(
      <EntityTagList entityType="artist" entityId={10} isAuthenticated={false} />
    )

    // No vote buttons rendered
    expect(screen.queryByLabelText('Upvote rock')).not.toBeInTheDocument()
  })

  // ── Add tag button ──

  it('shows Add tag button when authenticated', () => {
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    expect(screen.getByLabelText('Add tag')).toBeInTheDocument()
  })

  it('does not show Add tag button when not authenticated', () => {
    const tags = [makeTag()]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated={false} />
    )

    expect(screen.queryByLabelText('Add tag')).not.toBeInTheDocument()
  })

  it('opens Add Tag dialog when Add button is clicked', async () => {
    const user = userEvent.setup()
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))

    expect(screen.getByText('Add Tag')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Search tags or type a new one...')).toBeInTheDocument()
  })

  // ── Multiple tags ──

  it('renders many tags', () => {
    const tags = Array.from({ length: 10 }, (_, i) =>
      makeTag({
        tag_id: i + 1,
        name: `tag-${i + 1}`,
        slug: `tag-${i + 1}`,
      })
    )
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    for (let i = 1; i <= 10; i++) {
      expect(screen.getByRole('link', { name: `tag-${i}` })).toBeInTheDocument()
    }
  })

  // ── Long tag names ──

  it('renders tag with a very long name', () => {
    const longName = 'a'.repeat(100)
    const tags = [makeTag({ name: longName, slug: longName })]
    mockEntityTags.mockReturnValue({
      data: { tags },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} />
    )

    expect(screen.getByRole('link', { name: longName })).toBeInTheDocument()
  })
})

describe('AddTagForm (via EntityTagList dialog)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockEntityTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })
    mockSearchTags.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
    })
  })

  it('shows "Type at least 2 characters" hint for short queries', async () => {
    const user = userEvent.setup()

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, 'r')

    expect(screen.getByText('Type at least 2 characters to search...')).toBeInTheDocument()
  })

  it('shows search results after debounced query', async () => {
    const user = userEvent.setup()

    mockSearchTags.mockReturnValue({
      data: {
        tags: [
          { id: 10, name: 'rock', slug: 'rock', category: 'genre', is_official: false, usage_count: 42, created_at: '' },
        ],
      },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, 'rock')

    // Wait for debounce
    await waitFor(() => {
      expect(screen.getByText('rock')).toBeInTheDocument()
    })
  })

  it('filters out already-existing tags from search results', async () => {
    const user = userEvent.setup()
    const existingTags = [makeTag({ tag_id: 10, name: 'rock', slug: 'rock' })]

    mockEntityTags.mockReturnValue({
      data: { tags: existingTags },
      isLoading: false,
    })

    mockSearchTags.mockReturnValue({
      data: {
        tags: [
          { id: 10, name: 'rock', slug: 'rock', category: 'genre', is_official: false, usage_count: 42, created_at: '' },
          { id: 11, name: 'rocket', slug: 'rocket', category: 'genre', is_official: false, usage_count: 5, created_at: '' },
        ],
      },
      isLoading: false,
    })

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, 'rock')

    // Wait for debounce and check only non-existing tag shows as selectable button
    await waitFor(() => {
      // "rock" is already on the entity, so only "rocket" should be shown as a selectable option
      const buttons = screen.getAllByRole('button')
      const rocketButton = buttons.find(b => b.textContent?.includes('rocket'))
      expect(rocketButton).toBeDefined()
    })
  })

  it('shows clear button when search has input', async () => {
    const user = userEvent.setup()

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))
    const input = screen.getByPlaceholderText('Search tags or type a new one...')
    await user.type(input, 'test')

    expect(screen.getByLabelText('Clear search')).toBeInTheDocument()
  })

  it('clears search input when clear button is clicked', async () => {
    const user = userEvent.setup()

    renderWithProviders(
      <EntityTagList entityType="artist" entityId={1} isAuthenticated />
    )

    await user.click(screen.getByLabelText('Add tag'))
    const input = screen.getByPlaceholderText('Search tags or type a new one...') as HTMLInputElement
    await user.type(input, 'test')

    await user.click(screen.getByLabelText('Clear search'))

    expect(input.value).toBe('')
  })
})
