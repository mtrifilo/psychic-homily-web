import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Request, RequestListResponse } from '../types'

// ── Mocks ──────────────────────────────────────────

type MockAuthValue = {
  user: { id: string; is_admin?: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthValue>(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const mockUseRequests = vi.fn()
const mockCreateMutate = vi.fn()
const mockCreateMutation = vi.fn(() => ({
  mutate: mockCreateMutate,
  isPending: false,
  error: null,
}))
const mockRefetch = vi.fn()
vi.mock('../hooks', () => ({
  useRequests: (params: unknown) => mockUseRequests(params),
  useCreateRequest: () => mockCreateMutation(),
}))

// Stub RequestCard so the list test does not pull in the card's own hooks.
vi.mock('./RequestCard', () => ({
  RequestCard: ({ request }: { request: Request }) => (
    <article data-testid={`request-card-${request.id}`}>{request.title}</article>
  ),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
    ...props
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button
      onClick={onClick}
      disabled={disabled}
      type={props.type as 'button' | 'reset' | 'submit' | undefined}
    >
      {children}
    </button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} />
  ),
}))

vi.mock('@/components/ui/textarea', () => ({
  Textarea: (props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => (
    <textarea {...props} />
  ),
}))

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) => (
    <div data-testid="dialog" data-open={open}>
      {children}
    </div>
  ),
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dialog-content">{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: React.ReactNode }) => (
    <h2>{children}</h2>
  ),
  DialogTrigger: ({ children }: { children: React.ReactNode; asChild?: boolean }) => (
    <>{children}</>
  ),
}))

import { RequestList } from './RequestList'

function makeRequest(overrides: Partial<Request> = {}): Request {
  return {
    id: 1,
    title: 'Add Slowdive',
    entity_type: 'artist',
    status: 'pending',
    requester_id: 1,
    requester_name: 'jane',
    requester_username: 'jane',
    vote_score: 3,
    upvotes: 4,
    downvotes: 1,
    wilson_score: 0.5,
    user_vote: null,
    created_at: '2026-05-18T12:00:00Z',
    updated_at: '2026-05-18T12:00:00Z',
    ...overrides,
  }
}

function listResult(
  overrides: Partial<ReturnType<typeof mockUseRequests>> = {}
) {
  return {
    data: { requests: [], total: 0 } as RequestListResponse,
    isLoading: false,
    error: null as Error | null,
    refetch: mockRefetch,
    ...overrides,
  }
}

describe('RequestList', () => {
  beforeEach(() => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockCreateMutation.mockReturnValue({
      mutate: mockCreateMutate,
      isPending: false,
      error: null,
    })
    mockUseRequests.mockReturnValue(listResult())
  })

  it('shows a loading spinner before the first response', () => {
    mockUseRequests.mockReturnValue(
      listResult({ isLoading: true, data: undefined })
    )
    render(<RequestList />)
    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
  })

  it('shows an error state with a retry button', async () => {
    const user = userEvent.setup()
    mockUseRequests.mockReturnValue(
      listResult({ error: new Error('boom'), data: undefined })
    )
    render(<RequestList />)

    expect(
      screen.getByText(/Failed to load requests/i)
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalled()
  })

  it('renders a card per request and a results count', () => {
    mockUseRequests.mockReturnValue(
      listResult({
        data: {
          requests: [
            makeRequest({ id: 1, title: 'Add Slowdive' }),
            makeRequest({ id: 2, title: 'Add Ride' }),
          ],
          total: 2,
        },
      })
    )
    render(<RequestList />)

    expect(screen.getByTestId('request-card-1')).toBeInTheDocument()
    expect(screen.getByTestId('request-card-2')).toBeInTheDocument()
    expect(screen.getByText('2 requests found')).toBeInTheDocument()
  })

  it('uses the singular noun for a single result', () => {
    mockUseRequests.mockReturnValue(
      listResult({ data: { requests: [makeRequest()], total: 1 } })
    )
    render(<RequestList />)
    expect(screen.getByText('1 request found')).toBeInTheDocument()
  })

  it('renders an empty state when there are no requests', () => {
    render(<RequestList />)
    expect(screen.getByText('No requests found.')).toBeInTheDocument()
  })

  it('passes the votes sort to useRequests by default', () => {
    render(<RequestList />)
    expect(mockUseRequests).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: 'votes', limit: 20, offset: 0 })
    )
  })

  it('forwards the selected entity-type filter to useRequests', async () => {
    const user = userEvent.setup()
    render(<RequestList />)

    await user.selectOptions(
      screen.getByLabelText('Filter by entity type'),
      'venue'
    )
    expect(mockUseRequests).toHaveBeenLastCalledWith(
      expect.objectContaining({ entity_type: 'venue' })
    )
  })

  it('forwards the selected status filter to useRequests', async () => {
    const user = userEvent.setup()
    render(<RequestList />)

    await user.selectOptions(
      screen.getByLabelText('Filter by status'),
      'fulfilled'
    )
    expect(mockUseRequests).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'fulfilled' })
    )
  })

  it('forwards the chosen sort to useRequests', async () => {
    const user = userEvent.setup()
    render(<RequestList />)

    await user.selectOptions(screen.getByLabelText('Sort by'), 'newest')
    expect(mockUseRequests).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'newest' })
    )
  })

  describe('authentication-gated UI', () => {
    it('hides the New Request action when signed out', () => {
      render(<RequestList />)
      expect(
        screen.queryByRole('button', { name: /New Request/i })
      ).not.toBeInTheDocument()
    })

    it('shows the New Request action when signed in', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestList />)
      expect(
        screen.getByRole('button', { name: /New Request/i })
      ).toBeInTheDocument()
    })
  })

  describe('pagination', () => {
    it('hides pagination on a single page', () => {
      mockUseRequests.mockReturnValue(
        listResult({ data: { requests: [makeRequest()], total: 5 } })
      )
      render(<RequestList />)
      expect(
        screen.queryByRole('button', { name: 'Next' })
      ).not.toBeInTheDocument()
    })

    it('advances the offset when Next is clicked', async () => {
      const user = userEvent.setup()
      mockUseRequests.mockReturnValue(
        listResult({
          data: { requests: [makeRequest()], total: 40 },
        })
      )
      render(<RequestList />)

      await user.click(screen.getByRole('button', { name: 'Next' }))
      expect(mockUseRequests).toHaveBeenLastCalledWith(
        expect.objectContaining({ offset: 20 })
      )
    })
  })

  describe('create form', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
    })

    it('submits a new request with trimmed title and entity type', async () => {
      const user = userEvent.setup()
      render(<RequestList />)

      const dialog = screen.getByTestId('dialog-content')
      await user.type(within(dialog).getByLabelText('Title'), '  New Band  ')
      await user.click(
        within(dialog).getByRole('button', { name: 'Create Request' })
      )

      expect(mockCreateMutate).toHaveBeenCalledWith(
        { title: 'New Band', description: undefined, entity_type: 'artist' },
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('does not submit when the title is blank', async () => {
      const user = userEvent.setup()
      render(<RequestList />)

      const dialog = screen.getByTestId('dialog-content')
      const submit = within(dialog).getByRole('button', {
        name: 'Create Request',
      })
      // Empty title keeps the submit disabled; clicking is a no-op.
      expect(submit).toBeDisabled()
      await user.click(submit)
      expect(mockCreateMutate).not.toHaveBeenCalled()
    })
  })
})
