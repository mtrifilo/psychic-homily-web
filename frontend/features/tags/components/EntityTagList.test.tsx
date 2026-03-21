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
    { tag_id: 1, name: 'rock', slug: 'rock', category: 'genre', upvotes: 3, downvotes: 0, user_vote: 0 },
    { tag_id: 2, name: 'indie', slug: 'indie', category: 'genre', upvotes: 1, downvotes: 0, user_vote: 0 },
  ],
}

const mockSearchTags = {
  tags: [
    { id: 3, name: 'punk', slug: 'punk', category: 'genre', usage_count: 5 },
  ],
}

vi.mock('../hooks', () => ({
  useEntityTags: () => ({
    data: mockEntityTags,
    isLoading: false,
  }),
  useAddTagToEntity: () => ({
    mutate: vi.fn(),
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
    data: mockSearchTags,
    isLoading: false,
  }),
}))

vi.mock('../types', () => ({
  getCategoryColor: () => '',
}))

import { EntityTagList } from './EntityTagList'

describe('EntityTagList add-tag dialog accessibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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
})
