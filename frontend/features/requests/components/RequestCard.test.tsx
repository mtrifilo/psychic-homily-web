import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Request } from '../types'

// ── Mocks ──────────────────────────────────────────

type MockAuthValue = {
  user: { id: string; is_admin?: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthValue>(() => ({
  user: { id: '1' },
  isAuthenticated: true,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const mockVoteMutate = vi.fn()
const mockRemoveVoteMutate = vi.fn()
const mockVoteMutation = vi.fn(() => ({
  mutate: mockVoteMutate,
  isPending: false,
}))
const mockRemoveVoteMutation = vi.fn(() => ({
  mutate: mockRemoveVoteMutate,
  isPending: false,
}))
vi.mock('../hooks', () => ({
  useVoteRequest: () => mockVoteMutation(),
  useRemoveVoteRequest: () => mockRemoveVoteMutation(),
}))

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

import { RequestCard } from './RequestCard'

function makeRequest(overrides: Partial<Request> = {}): Request {
  return {
    id: 42,
    title: 'Add Slowdive discography',
    description: 'Shoegaze legends, missing a few EPs.',
    entity_type: 'artist',
    status: 'pending',
    requester_id: 1,
    requester_name: 'jane',
    requester_username: 'jane',
    vote_score: 5,
    upvotes: 7,
    downvotes: 2,
    wilson_score: 0.5,
    user_vote: null,
    created_at: '2026-05-18T12:00:00Z',
    updated_at: '2026-05-18T12:00:00Z',
    ...overrides,
  }
}

describe('RequestCard', () => {
  beforeEach(() => {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockVoteMutation.mockReturnValue({ mutate: mockVoteMutate, isPending: false })
    mockRemoveVoteMutation.mockReturnValue({
      mutate: mockRemoveVoteMutate,
      isPending: false,
    })
  })

  it('renders as an article element', () => {
    render(<RequestCard request={makeRequest()} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('renders the title linked to the request detail page', () => {
    render(<RequestCard request={makeRequest()} />)
    const titleLink = screen
      .getByText('Add Slowdive discography')
      .closest('a')
    expect(titleLink).toHaveAttribute('href', '/requests/42')
  })

  it('renders the entity-type and status badges', () => {
    render(<RequestCard request={makeRequest()} />)
    expect(screen.getByText('Artist')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
  })

  it('renders the in-progress status label for an in_progress request', () => {
    render(<RequestCard request={makeRequest({ status: 'in_progress' })} />)
    expect(screen.getByText('In Progress')).toBeInTheDocument()
  })

  it('renders the vote score', () => {
    render(<RequestCard request={makeRequest({ vote_score: 5 })} />)
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  it('renders the description when present', () => {
    render(<RequestCard request={makeRequest()} />)
    expect(
      screen.getByText('Shoegaze legends, missing a few EPs.')
    ).toBeInTheDocument()
  })

  it('omits the description paragraph when absent', () => {
    render(<RequestCard request={makeRequest({ description: undefined })} />)
    expect(
      screen.queryByText('Shoegaze legends, missing a few EPs.')
    ).not.toBeInTheDocument()
  })

  it('renders the requester attribution', () => {
    render(<RequestCard request={makeRequest()} />)
    // UserAttribution links to the profile when username is set.
    const byline = screen.getByText('jane').closest('a')
    expect(byline).toHaveAttribute('href', '/users/jane')
  })

  it('renders requester as plain text when username is null', () => {
    render(
      <RequestCard
        request={makeRequest({
          requester_name: 'jane',
          requester_username: null,
        })}
      />
    )
    expect(screen.getByText('jane').closest('a')).toBeNull()
  })

  describe('voting', () => {
    it('casts an upvote when no prior vote exists', async () => {
      const user = userEvent.setup()
      render(<RequestCard request={makeRequest({ user_vote: null })} />)

      await user.click(screen.getByRole('button', { name: 'Upvote' }))
      expect(mockVoteMutate).toHaveBeenCalledWith({
        requestId: 42,
        is_upvote: true,
      })
      expect(mockRemoveVoteMutate).not.toHaveBeenCalled()
    })

    it('removes the vote when re-clicking an existing upvote', async () => {
      const user = userEvent.setup()
      render(<RequestCard request={makeRequest({ user_vote: 1 })} />)

      await user.click(screen.getByRole('button', { name: 'Upvote' }))
      expect(mockRemoveVoteMutate).toHaveBeenCalledWith({ requestId: 42 })
      expect(mockVoteMutate).not.toHaveBeenCalled()
    })

    it('casts a downvote when no prior vote exists', async () => {
      const user = userEvent.setup()
      render(<RequestCard request={makeRequest({ user_vote: null })} />)

      await user.click(screen.getByRole('button', { name: 'Downvote' }))
      expect(mockVoteMutate).toHaveBeenCalledWith({
        requestId: 42,
        is_upvote: false,
      })
    })

    it('removes the vote when re-clicking an existing downvote', async () => {
      const user = userEvent.setup()
      render(<RequestCard request={makeRequest({ user_vote: -1 })} />)

      await user.click(screen.getByRole('button', { name: 'Downvote' }))
      expect(mockRemoveVoteMutate).toHaveBeenCalledWith({ requestId: 42 })
    })

    it('disables vote buttons and does not mutate when unauthenticated', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestCard request={makeRequest()} />)

      const upvote = screen.getByRole('button', { name: 'Upvote' })
      expect(upvote).toBeDisabled()

      await user.click(upvote)
      expect(mockVoteMutate).not.toHaveBeenCalled()
      expect(mockRemoveVoteMutate).not.toHaveBeenCalled()
    })

    it('disables both vote buttons while a vote is pending', () => {
      mockVoteMutation.mockReturnValue({
        mutate: mockVoteMutate,
        isPending: true,
      })
      render(<RequestCard request={makeRequest()} />)
      expect(screen.getByRole('button', { name: 'Upvote' })).toBeDisabled()
      expect(screen.getByRole('button', { name: 'Downvote' })).toBeDisabled()
    })
  })
})
