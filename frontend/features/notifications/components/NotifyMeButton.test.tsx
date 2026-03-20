import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationFilter } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockRouterPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockRouterPush }),
}))

const mockAuthContext = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

const mockFilterCheck = vi.fn()
const mockQuickCreateMutate = vi.fn()
const mockDeleteMutate = vi.fn()
const mockQuickCreateIsPending = vi.fn(() => false)
const mockDeleteIsPending = vi.fn(() => false)

vi.mock('../hooks', () => ({
  useNotificationFilterCheck: (...args: unknown[]) => mockFilterCheck(...args),
  useQuickCreateFilter: () => ({
    mutate: mockQuickCreateMutate,
    isPending: mockQuickCreateIsPending(),
  }),
  useDeleteFilter: () => ({
    mutate: mockDeleteMutate,
    isPending: mockDeleteIsPending(),
  }),
}))

import { NotifyMeButton } from './NotifyMeButton'

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

function makeFilter(overrides: Partial<NotificationFilter> = {}): NotificationFilter {
  return {
    id: 1,
    name: 'Auto filter',
    is_active: true,
    artist_ids: [10],
    venue_ids: null,
    label_ids: null,
    tag_ids: null,
    exclude_tag_ids: null,
    cities: null,
    price_max_cents: null,
    notify_email: true,
    notify_in_app: true,
    notify_push: false,
    match_count: 0,
    last_matched_at: null,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('NotifyMeButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockQuickCreateIsPending.mockReturnValue(false)
    mockDeleteIsPending.mockReturnValue(false)
  })

  // ── Unauthenticated ──

  it('renders "Notify me" button for unauthenticated users', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: false })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('redirects to /auth when unauthenticated user clicks (normal mode)', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: false })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    await user.click(screen.getByText('Notify me'))

    expect(mockRouterPush).toHaveBeenCalledWith('/auth')
  })

  it('renders compact bell icon for unauthenticated users in compact mode', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: false })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="artist"
        entityId={10}
        entityName="Gatecreeper"
        compact
      />
    )

    // In compact mode, button has title instead of visible text
    expect(
      screen.getByTitle('Sign in to get notifications')
    ).toBeInTheDocument()
  })

  it('redirects to /auth when compact unauthenticated user clicks', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: false })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="artist"
        entityId={10}
        entityName="Gatecreeper"
        compact
      />
    )

    await user.click(screen.getByTitle('Sign in to get notifications'))

    expect(mockRouterPush).toHaveBeenCalledWith('/auth')
  })

  // ── Authenticated, loading ──

  it('shows loading spinner while checking for existing filter', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: true,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('disables button while loading', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: true,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    // Both buttons (normal loading state renders a disabled button)
    const buttons = screen.getAllByRole('button')
    buttons.forEach(b => expect(b).toBeDisabled())
  })

  it('shows loading spinner in compact mode while checking', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: true,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="artist"
        entityId={10}
        entityName="Gatecreeper"
        compact
      />
    )

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Authenticated, no existing filter ──

  it('renders "Notify me" when no matching filter exists', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('calls quickCreate.mutate when "Notify me" is clicked', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    await user.click(screen.getByText('Notify me'))

    expect(mockQuickCreateMutate).toHaveBeenCalledWith({
      entityType: 'artist',
      entityId: 10,
    })
  })

  // ── Authenticated, has existing filter ──

  it('renders "Notifications on" when a matching filter exists', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: makeFilter({ id: 42 }),
      hasFilter: true,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    expect(screen.getByText('Notifications on')).toBeInTheDocument()
  })

  it('calls deleteFilter.mutate when clicking existing filter button', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: makeFilter({ id: 42 }),
      hasFilter: true,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    await user.click(screen.getByText('Notifications on'))

    expect(mockDeleteMutate).toHaveBeenCalledWith(42)
  })

  // ── Compact mode (authenticated) ──

  it('renders compact bell icon when no filter exists', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="artist"
        entityId={10}
        entityName="Gatecreeper"
        compact
      />
    )

    expect(screen.getByLabelText('Notify me')).toBeInTheDocument()
  })

  it('renders compact bell with title when filter exists', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: makeFilter(),
      hasFilter: true,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="artist"
        entityId={10}
        entityName="Gatecreeper"
        compact
      />
    )

    expect(
      screen.getByTitle('Notifications on for Gatecreeper')
    ).toBeInTheDocument()
  })

  it('quick-creates filter when compact no-filter button is clicked', async () => {
    const user = userEvent.setup()
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton
        entityType="venue"
        entityId={20}
        entityName="The Rebel Lounge"
        compact
      />
    )

    await user.click(screen.getByLabelText('Notify me'))

    expect(mockQuickCreateMutate).toHaveBeenCalledWith({
      entityType: 'venue',
      entityId: 20,
    })
  })

  // ── Disabled during mutations ──

  it('disables button during quickCreate mutation', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })
    mockQuickCreateIsPending.mockReturnValue(true)

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    const button = screen.getByRole('button')
    expect(button).toBeDisabled()
  })

  it('disables button during deleteFilter mutation', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: makeFilter({ id: 42 }),
      hasFilter: true,
      isLoading: false,
    })
    mockDeleteIsPending.mockReturnValue(true)

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    const button = screen.getByRole('button')
    expect(button).toBeDisabled()
  })

  it('shows spinner during mutation', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })
    mockQuickCreateIsPending.mockReturnValue(true)

    renderWithProviders(
      <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
    )

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Different entity types ──

  it('works for venue entity type', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="venue" entityId={20} entityName="The Rebel Lounge" />
    )

    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('works for label entity type', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="label" entityId={30} entityName="Run For Cover" />
    )

    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('works for tag entity type', () => {
    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    renderWithProviders(
      <NotifyMeButton entityType="tag" entityId={40} entityName="punk" />
    )

    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  // ── Event propagation ──

  it('stops event propagation on click', async () => {
    const user = userEvent.setup()
    const parentClickHandler = vi.fn()

    mockAuthContext.mockReturnValue({ isAuthenticated: true })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
    })

    render(
      <QueryClientProvider client={createQueryClient()}>
        {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
        <div onClick={parentClickHandler}>
          <NotifyMeButton entityType="artist" entityId={10} entityName="Gatecreeper" />
        </div>
      </QueryClientProvider>
    )

    await user.click(screen.getByText('Notify me'))

    // stopPropagation should prevent the parent handler from firing
    expect(parentClickHandler).not.toHaveBeenCalled()
  })
})
