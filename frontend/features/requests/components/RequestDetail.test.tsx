import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Request } from '../types'
import type { EntitySearchResults } from '@/lib/hooks/common/useEntitySearch'

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
  // PSY-917: FulfillmentEntityPicker renders InlineErrorBanner on a search
  // outage. Provide a passthrough so the picker mounts in these tests.
  InlineErrorBanner: ({ children }: { children: React.ReactNode }) => (
    <div role="alert">{children}</div>
  ),
}))

// PSY-917: the propose-fulfillment picker reuses the shared entity-search
// hook. Mock it so the dialog renders deterministically without real network
// calls — individual tests override the return value to drive results.
type EntitySearchStub = {
  data: EntitySearchResults
  isSearching: boolean
  searchError: boolean
}
const emptyEntitySearchResults: EntitySearchResults = {
  artists: [],
  venues: [],
  shows: [],
  releases: [],
  labels: [],
  festivals: [],
  tags: [],
}
const mockUseEntitySearch = vi.fn<() => EntitySearchStub>(() => ({
  data: emptyEntitySearchResults,
  isSearching: false,
  searchError: false,
}))
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockUseEntitySearch(),
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE: 'Search is temporarily unavailable.',
}))

// Hooks — query + mutation stubs. The mutation factories are reset per-test
// so individual cases can flip isPending without leaking across tests.
const mockUseRequest = vi.fn()

const mockDeleteMutate = vi.fn()
const mockVoteMutate = vi.fn()
const mockRemoveVoteMutate = vi.fn()
const mockFulfillMutate = vi.fn()
const mockApproveMutate = vi.fn()
const mockRejectMutate = vi.fn()
const mockCloseMutate = vi.fn()
const mockUpdateMutate = vi.fn()

type MutationStub = {
  mutate: ReturnType<typeof vi.fn>
  isPending: boolean
  isError?: boolean
  error?: Error | null
  reset?: ReturnType<typeof vi.fn>
}

// PSY-917: openProposeModal() calls fulfillMutation.reset() to clear stale
// error state before opening the picker dialog, so the fulfill stub needs a
// no-op reset. Shared so the factory + beforeEach agree.
const mockFulfillReset = vi.fn()
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
  error: null,
  reset: mockFulfillReset,
}))
const mockApproveMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockApproveMutate,
  isPending: false,
  error: null,
}))
const mockRejectMutation = vi.fn<() => MutationStub>(() => ({
  mutate: mockRejectMutate,
  isPending: false,
  error: null,
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
  useApproveFulfillment: () => mockApproveMutation(),
  useRejectFulfillment: () => mockRejectMutation(),
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

// The delete action is icon-only but carries an aria-label so screen readers
// can announce it. Querying by role+name is the authoritative shape.
function getDeleteButton(): HTMLElement {
  return screen.getByRole('button', { name: /delete request/i })
}

function queryResult(
  overrides: Partial<ReturnType<typeof mockUseRequest>> = {}
) {
  return {
    data: makeRequest(),
    isLoading: false,
    error: null as Error | null,
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
      error: null,
      reset: mockFulfillReset,
    })
    mockApproveMutation.mockReturnValue({
      mutate: mockApproveMutate,
      isPending: false,
      error: null,
    })
    mockRejectMutation.mockReturnValue({
      mutate: mockRejectMutate,
      isPending: false,
      error: null,
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
    mockUseEntitySearch.mockReturnValue({
      data: emptyEntitySearchResults,
      isSearching: false,
      searchError: false,
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

    it('links to the requested entity by slug when one is attached (PSY-917)', () => {
      // Entity pages route by slug, not id, so the link must use the
      // server-resolved requested_entity_slug. The name is the link label.
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({
            entity_type: 'artist',
            requested_entity_id: 99,
            requested_entity_slug: 'slowdive',
            requested_entity_name: 'Slowdive',
          }),
        })
      )
      render(<RequestDetail requestId={42} />)
      const link = screen.getByText(/View requested Slowdive/i).closest('a')
      expect(link).toHaveAttribute('href', '/artists/slowdive')
    })

    it('suppresses the entity link when no slug resolved (PSY-917)', () => {
      // A legacy request with only an id (no slug) must NOT render a dead
      // /artists/<id> link.
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({
            entity_type: 'artist',
            requested_entity_id: 99,
            requested_entity_slug: null,
          }),
        })
      )
      render(<RequestDetail requestId={42} />)
      expect(screen.queryByText(/View requested/i)).not.toBeInTheDocument()
    })

    it('shows a "View proposed {entity}" link in the review panel (PSY-917)', () => {
      // Requester reviewing a pending_fulfillment proposal sees a link to the
      // proposed entity, keyed off the resolved slug.
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({
            entity_type: 'artist',
            status: 'pending_fulfillment',
            fulfiller_name: 'contributor-cara',
            requested_entity_id: 99,
            requested_entity_slug: 'slowdive',
            requested_entity_name: 'Slowdive',
          }),
        })
      )
      render(<RequestDetail requestId={42} />)
      const link = screen
        .getByTestId('review-panel-proposed-entity-link')
        .closest('a')
      expect(link).toHaveAttribute('href', '/artists/slowdive')
      expect(link).toHaveTextContent(/View proposed Slowdive/i)
      // The main entity-link block is suppressed for the requester while the
      // review panel owns the link — so exactly ONE "View proposed" link
      // renders, not two.
      expect(screen.getAllByText(/View proposed Slowdive/i)).toHaveLength(1)
    })

    it('shows the proposed link in the main block for a non-reviewer (PSY-917)', () => {
      // A viewer who is NOT the requester/admin sees the proposed entity via
      // the main block (no review panel for them).
      mockAuthContext.mockReturnValue({
        user: { id: '77', is_admin: false },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseRequest.mockReturnValue(
        queryResult({
          data: makeRequest({
            entity_type: 'artist',
            status: 'pending_fulfillment',
            requester_id: 1,
            fulfiller_name: 'contributor-cara',
            requested_entity_id: 99,
            requested_entity_slug: 'slowdive',
            requested_entity_name: 'Slowdive',
          }),
        })
      )
      render(<RequestDetail requestId={42} />)
      // No review panel for a non-reviewer; the main block carries the link.
      expect(
        screen.queryByTestId('review-panel-proposed-entity-link')
      ).not.toBeInTheDocument()
      const link = screen.getByText(/View proposed Slowdive/i).closest('a')
      expect(link).toHaveAttribute('href', '/artists/slowdive')
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
      // Close is admin-only and must stay hidden for a plain requester. (Note:
      // "Propose a fulfillment" DOES show for the requester on an open request
      // — submit is open to any authed user post-PSY-748 — so we assert on
      // Close, the genuinely admin-only action, rather than /Fulfill/i.)
      expect(
        screen.queryByRole('button', { name: /Close/i })
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

    it('opens the entity picker instead of submitting immediately (PSY-917)', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)

      // Clicking "Propose a fulfillment" must NOT submit — it opens the
      // mandatory-entity picker. The mutation only fires after a pick.
      await user.click(
        screen.getByRole('button', { name: /Propose a fulfillment/i })
      )
      expect(mockFulfillMutate).not.toHaveBeenCalled()
      expect(
        screen.getByTestId('fulfillment-entity-picker')
      ).toBeInTheDocument()
    })

    it('submits the picked entity id as fulfilled_entity_id (PSY-917)', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      // One artist result so the picker has something to select.
      mockUseEntitySearch.mockReturnValue({
        data: {
          ...emptyEntitySearchResults,
          artists: [
            {
              id: 555,
              slug: 'slowdive',
              name: 'Slowdive',
              subtitle: 'Reading, UK',
              entityType: 'artist',
              href: '/artists/slowdive',
            },
          ],
        },
        isSearching: false,
        searchError: false,
      })
      render(<RequestDetail requestId={42} />)

      await user.click(
        screen.getByRole('button', { name: /Propose a fulfillment/i })
      )
      // Type to trigger the results render (the picker gates rows behind a
      // 2-char query even though the hook is mocked).
      await user.type(
        screen.getByTestId('fulfillment-entity-picker-search-input'),
        'slow'
      )
      await user.click(screen.getByTestId('fulfillment-entity-picker-result-row'))
      await user.click(
        screen.getByTestId('fulfillment-entity-picker-confirm')
      )

      expect(mockFulfillMutate).toHaveBeenCalledWith(
        { requestId: 42, fulfilled_entity_id: 555 },
        expect.objectContaining({ onSuccess: expect.any(Function) })
      )
    })

    it('disables the confirm button until an entity is picked (PSY-917)', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      render(<RequestDetail requestId={42} />)

      await user.click(
        screen.getByRole('button', { name: /Propose a fulfillment/i })
      )
      // Nothing selected yet — confirm is disabled (mandatory picker).
      expect(
        screen.getByTestId('fulfillment-entity-picker-confirm')
      ).toBeDisabled()
    })

    it('surfaces the backend type-mismatch error inline in the picker (PSY-917)', async () => {
      const user = userEvent.setup()
      mockAuthContext.mockReturnValue({
        user: { id: '99', is_admin: true },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      // Fulfill mutation is in an error state (e.g. PSY-748 400 mismatch).
      mockFulfillMutation.mockReturnValue({
        mutate: mockFulfillMutate,
        isPending: false,
        error: new Error('Entity type does not match the request'),
        reset: mockFulfillReset,
      })
      render(<RequestDetail requestId={42} />)

      await user.click(
        screen.getByRole('button', { name: /Propose a fulfillment/i })
      )
      expect(
        screen.getByTestId('fulfillment-entity-picker-submit-error')
      ).toHaveTextContent('Entity type does not match the request')
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
