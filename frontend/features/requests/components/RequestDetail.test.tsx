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

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
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

vi.mock('@/components/shared', () => ({
  Breadcrumb: ({ currentPage }: { currentPage: string }) => (
    <nav aria-label="Breadcrumb">
      <span data-testid="breadcrumb-current">{currentPage}</span>
    </nav>
  ),
  UserAttribution: ({
    name,
    username,
  }: {
    name?: string | null
    username?: string | null
    className?: string
  }) =>
    username ? (
      <a href={`/users/${username}`}>{name}</a>
    ) : (
      <span>{name}</span>
    ),
}))

// Hooks — query + mutation stubs. The mutation factories are reset per-test
// so individual cases can flip isPending without leaking across tests.
const mockUseRequest = vi.fn()

const mockDeleteMutate = vi.fn()
const mockVoteMutate = vi.fn()
const mockRemoveVoteMutate = vi.fn()
const mockFulfillMutate = vi.fn()
const mockCloseMutate = vi.fn()
const mockUpdateMutate = vi.fn()

type MutationStub = {
  mutate: ReturnType<typeof vi.fn>
  isPending: boolean
  isError?: boolean
  error?: Error | null
}
const mockDeleteMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockDeleteMutate,
  isPending: false,
}))
const mockVoteMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockVoteMutate,
  isPending: false,
}))
const mockRemoveVoteMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockRemoveVoteMutate,
  isPending: false,
}))
const mockFulfillMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockFulfillMutate,
  isPending: false,
}))
const mockCloseMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockCloseMutate,
  isPending: false,
}))
const mockUpdateMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockUpdateMutate,
  isPending: false,
  error: null,
}))

vi.mock('../hooks', () => ({
  useRequest: (id: number) => mockUseRequest(id),
  useUpdateRequest: () => mockUpdateMutation(),
  useDeleteRequest: () => mockDeleteMutation(),
  useVoteRequest: () => mockVoteMutation(),
  useRemoveVoteRequest: () => mockRemoveVoteMutation(),
  useFulfillRequest: () => mockFulfillMutation(),
  useCloseRequest: () => mockCloseMutation(),
}))

import { RequestDetail } from './RequestDetail'

function makeRequest(overrides: Partial<Request> = {}): Request {
  return {
    id: 42,
    title: 'Add Slowdive discography',
    description: 'Shoegaze legends.',
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

// The delete action is an icon-only button with no accessible name, so it
// can't be reached via getByRole name. It is the only action styled with the
// destructive class, which is the stable hook the test targets.
function getDeleteButton(): HTMLElement {
  const deleteBtn = screen
    .getAllByRole('button')
    .find(b => b.className.includes('text-destructive'))
  if (!deleteBtn) throw new Error('delete button not found')
  return deleteBtn
}

function queryResult(
  overrides: Partial<ReturnType<typeof mockUseRequest>> = {}
) {
  return {
    data: makeRequest(),
    isLoading: false,
    error: null,
    ...overrides,
  }
}

describe('RequestDetail', () => {
  beforeEach(() => {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseRequest.mockReturnValue(queryResult())
    mockDeleteMutation.mockReturnValue({
      mutate: mockDeleteMutate,
      isPending: false,
    })
    mockVoteMutation.mockReturnValue({ mutate: mockVoteMutate, isPending: false })
    mockRemoveVoteMutation.mockReturnValue({
      mutate: mockRemoveVoteMutate,
      isPending: false,
    })
    mockFulfillMutation.mockReturnValue({
      mutate: mockFulfillMutate,
      isPending: false,
    })
    mockCloseMutation.mockReturnValue({
      mutate: mockCloseMutate,
      isPending: false,
    })
    mockUpdateMutation.mockReturnValue({
      mutate: mockUpdateMutate,
      isPending: false,
      error: null,
    })
  })

  describe('loading and error states', () => {
    it('renders a spinner while loading', () => {
      mockUseRequest.mockReturnValue(
        queryResult({ isLoading: true, data: undefined })
      )
      const { container } = render(<RequestDetail requestId={42} />)
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
    })

    it('renders a not-found message for a 404 error', () => {
      mockUseRequest.mockReturnValue(
        queryResult({ error: new Error('not found'), data: undefined })
      )
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('Request Not Found')).toBeInTheDocument()
    })

    it('renders a generic error message for other errors', () => {
      mockUseRequest.mockReturnValue(
        queryResult({ error: new Error('server exploded'), data: undefined })
      )
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('Error Loading Request')).toBeInTheDocument()
      expect(screen.getByText('server exploded')).toBeInTheDocument()
    })

    it('renders a not-found message when the request is missing', () => {
      mockUseRequest.mockReturnValue(queryResult({ data: undefined }))
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('Request Not Found')).toBeInTheDocument()
    })
  })

  describe('rendering', () => {
    it('renders the title, status badge, and entity-type badge', () => {
      render(<RequestDetail requestId={42} />)
      expect(
        screen.getByRole('heading', { name: 'Add Slowdive discography' })
      ).toBeInTheDocument()
      expect(screen.getByText('Pending')).toBeInTheDocument()
      expect(screen.getByText('Artist')).toBeInTheDocument()
    })

    it('renders the vote score plus the up/down breakdown', () => {
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('5')).toBeInTheDocument()
      expect(screen.getByText('7 up / 2 down')).toBeInTheDocument()
    })

    it('renders the requester attribution as a profile link', () => {
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('jane').closest('a')).toHaveAttribute(
        'href',
        '/users/jane'
      )
    })

    it('renders the description', () => {
      render(<RequestDetail requestId={42} />)
      expect(screen.getByText('Shoegaze legends.')).toBeInTheDocument()
    })

    it('links to the requested entity when one is attached', () => {
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({ entity_type: 'artist', requested_entity_id: 99 }),
        })
      )
      render(<RequestDetail requestId={42} />)
      const link = screen
        .getByText(/View requested artist/i)
        .closest('a')
      expect(link).toHaveAttribute('href', '/artists/99')
    })

    it('renders fulfillment info for a fulfilled request', () => {
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({
            status: 'fulfilled',
            fulfiller_name: 'admin-bob',
            fulfilled_at: '2026-05-19T12:00:00Z',
          }),
        })
      )
      render(<RequestDetail requestId={42} />)
      // "Fulfilled" appears twice for a fulfilled request: the status badge
      // and the fulfillment info box. The fulfiller byline is unique.
      expect(screen.getAllByText('Fulfilled')).toHaveLength(2)
      expect(screen.getByText(/by admin-bob/)).toBeInTheDocument()
    })
  })

  describe('voting', () => {
    it('casts an upvote when no prior vote exists', async () => {
      const user = userEvent.setup()
      render(<RequestDetail requestId={42} />)
      await user.click(screen.getByRole('button', { name: 'Upvote' }))
      expect(mockVoteMutate).toHaveBeenCalledWith({
        requestId: 42,
        is_upvote: true,
      })
    })

    it('removes the vote when re-clicking an existing upvote', async () => {
      const user = userEvent.setup()
      mockUseRequest.mockReturnValue(
        queryResult({ data: makeRequest({ user_vote: 1 }) })
      )
      render(<RequestDetail requestId={42} />)
      await user.click(screen.getByRole('button', { name: 'Upvote' }))
      expect(mockRemoveVoteMutate).toHaveBeenCalledWith({ requestId: 42 })
    })

    it('disables vote buttons when unauthenticated', () => {
      mockAuthContext.mockReturnValue({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)
      expect(screen.getByRole('button', { name: 'Upvote' })).toBeDisabled()
      expect(screen.getByRole('button', { name: 'Downvote' })).toBeDisabled()
    })
  })

  describe('permission-gated actions', () => {
    it('shows Edit and Delete for the requester', () => {
      // requester_id 1 matches the authed user id '1'.
      render(<RequestDetail requestId={42} />)
      expect(
        screen.getByRole('button', { name: /Edit/i })
      ).toBeInTheDocument()
      expect(getDeleteButton()).toBeInTheDocument()
      // Fulfill/Close are admin-only and must stay hidden for a plain requester.
      expect(
        screen.queryByRole('button', { name: /Fulfill/i })
      ).not.toBeInTheDocument()
    })

    it('hides Edit/Delete for a non-owner non-admin', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)
      expect(
        screen.queryByRole('button', { name: /Edit/i })
      ).not.toBeInTheDocument()
    })

    it('shows admin-only Fulfill and Close on a pending request', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)
      expect(
        screen.getByRole('button', { name: /Fulfill/i })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /Close/i })
      ).toBeInTheDocument()
    })

    it('hides Fulfill once the request is already fulfilled', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseRequest.mockReturnValue(
        queryResult({ data: makeRequest({ status: 'fulfilled' }) })
      )
      render(<RequestDetail requestId={42} />)
      expect(
        screen.queryByRole('button', { name: /Fulfill/i })
      ).not.toBeInTheDocument()
    })
  })

  describe('mutating actions', () => {
    it('deletes and navigates away after confirmation', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi
        .spyOn(window, 'confirm')
        .mockReturnValue(true)
      // mutate calls onSuccess synchronously so we can assert the redirect.
      mockDeleteMutate.mockImplementation(
        (_vars, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.()
      )

      render(<RequestDetail requestId={42} />)
      await user.click(getDeleteButton())

      expect(confirmSpy).toHaveBeenCalled()
      expect(mockDeleteMutate).toHaveBeenCalledWith(
        { requestId: 42 },
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
      expect(mockPush).toHaveBeenCalledWith('/requests')
      confirmSpy.mockRestore()
    })

    it('does not delete when confirmation is dismissed', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi
        .spyOn(window, 'confirm')
        .mockReturnValue(false)

      render(<RequestDetail requestId={42} />)
      await user.click(getDeleteButton())

      expect(confirmSpy).toHaveBeenCalled()
      expect(mockDeleteMutate).not.toHaveBeenCalled()
      confirmSpy.mockRestore()
    })

    it('fulfills immediately without a confirm prompt', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm')
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)

      await user.click(screen.getByRole('button', { name: /Fulfill/i }))
      expect(mockFulfillMutate).toHaveBeenCalledWith({ requestId: 42 })
      expect(confirmSpy).not.toHaveBeenCalled()
      confirmSpy.mockRestore()
    })

    it('closes the request after confirmation', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi
        .spyOn(window, 'confirm')
        .mockReturnValue(true)
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)

      await user.click(screen.getByRole('button', { name: /Close/i }))
      expect(confirmSpy).toHaveBeenCalled()
      expect(mockCloseMutate).toHaveBeenCalledWith({ requestId: 42 })
      confirmSpy.mockRestore()
    })
  })

  describe('inline edit', () => {
    it('opens the inline edit form and saves an update', async () => {
      const user = userEvent.setup()
      mockUpdateMutate.mockImplementation(
        (_vars, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.()
      )
      render(<RequestDetail requestId={42} />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))

      const titleInput = screen.getByLabelText('Title')
      await user.clear(titleInput)
      await user.type(titleInput, 'Updated title')
      await user.click(screen.getByRole('button', { name: /Save/i }))

      expect(mockUpdateMutate).toHaveBeenCalledWith(
        {
          requestId: 42,
          title: 'Updated title',
          description: 'Shoegaze legends.',
        },
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('returns to the read view when editing is cancelled', async () => {
      const user = userEvent.setup()
      render(<RequestDetail requestId={42} />)

      await user.click(screen.getByRole('button', { name: /Edit/i }))
      expect(screen.getByLabelText('Title')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(screen.queryByLabelText('Title')).not.toBeInTheDocument()
      expect(
        screen.getByRole('heading', { name: 'Add Slowdive discography' })
      ).toBeInTheDocument()
    })
  })
})
