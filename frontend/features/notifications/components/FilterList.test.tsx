import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationFilter } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUseNotificationFilters = vi.fn()

vi.mock('../hooks', () => ({
  useNotificationFilters: () => mockUseNotificationFilters(),
  useUpdateFilter: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteFilter: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useCreateFilter: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
}))

// Mock FilterForm to avoid deep dependency chain
vi.mock('./FilterForm', () => ({
  FilterForm: ({
    open,
    filter,
  }: {
    open: boolean
    onOpenChange: (open: boolean) => void
    filter?: NotificationFilter
  }) =>
    open ? (
      <div data-testid={filter ? 'edit-form' : 'create-form'}>
        Filter Form ({filter ? 'Edit' : 'Create'})
      </div>
    ) : null,
}))

import { FilterList } from './FilterList'

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
    name: 'Test Filter',
    is_active: true,
    artist_ids: [1],
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

describe('FilterList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseNotificationFilters.mockReturnValue({
      data: { filters: [] },
      isLoading: false,
      error: null,
    })
  })

  // ── Loading state ──

  it('shows loading spinner while filters are loading', () => {
    mockUseNotificationFilters.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    renderWithProviders(<FilterList />)

    const spinner = document.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  // ── Error state ──

  it('shows error message on failure', () => {
    mockUseNotificationFilters.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Server error'),
    })

    renderWithProviders(<FilterList />)

    expect(
      screen.getByText('Failed to load notification filters. Please try again.')
    ).toBeInTheDocument()
  })

  // ── Empty state ──

  it('shows empty state when no filters exist', () => {
    mockUseNotificationFilters.mockReturnValue({
      data: { filters: [] },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<FilterList />)

    expect(screen.getByText('No notification filters')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Create your first filter/ })
    ).toBeInTheDocument()
  })

  // ── Header ──

  it('renders page title and description', () => {
    renderWithProviders(<FilterList />)

    expect(screen.getByText('Notification Filters')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Get notified when new shows matching your criteria are approved.'
      )
    ).toBeInTheDocument()
  })

  it('renders "New Filter" button', () => {
    renderWithProviders(<FilterList />)

    expect(
      screen.getByRole('button', { name: /New Filter/ })
    ).toBeInTheDocument()
  })

  // ── Filter list rendering ──

  it('renders filter cards when filters exist', () => {
    mockUseNotificationFilters.mockReturnValue({
      data: {
        filters: [
          makeFilter({ id: 1, name: 'PHX punk' }),
          makeFilter({ id: 2, name: 'Tucson jazz' }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<FilterList />)

    expect(screen.getByText('PHX punk')).toBeInTheDocument()
    expect(screen.getByText('Tucson jazz')).toBeInTheDocument()
  })

  it('does not show empty state when filters exist', () => {
    mockUseNotificationFilters.mockReturnValue({
      data: { filters: [makeFilter()] },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<FilterList />)

    expect(screen.queryByText('No notification filters')).not.toBeInTheDocument()
  })

  // ── Create form dialog ──

  it('opens create form when "New Filter" button is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FilterList />)

    await user.click(screen.getByRole('button', { name: /New Filter/ }))

    expect(screen.getByTestId('create-form')).toBeInTheDocument()
  })

  it('opens create form when empty-state create button is clicked', async () => {
    const user = userEvent.setup()
    mockUseNotificationFilters.mockReturnValue({
      data: { filters: [] },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<FilterList />)

    await user.click(
      screen.getByRole('button', { name: /Create your first filter/ })
    )

    expect(screen.getByTestId('create-form')).toBeInTheDocument()
  })
})
