import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationFilter } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockUpdateMutate = vi.fn()
const mockDeleteMutate = vi.fn()
const mockUpdateIsPending = vi.fn(() => false)
const mockDeleteIsPending = vi.fn(() => false)

vi.mock('../hooks', () => ({
  useUpdateFilter: () => ({
    mutate: mockUpdateMutate,
    isPending: mockUpdateIsPending(),
  }),
  useDeleteFilter: () => ({
    mutate: mockDeleteMutate,
    isPending: mockDeleteIsPending(),
  }),
}))

import { FilterCard } from './FilterCard'

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
    name: 'PHX punk shows',
    is_active: true,
    artist_ids: [1, 2],
    venue_ids: null,
    label_ids: null,
    tag_ids: null,
    exclude_tag_ids: null,
    cities: null,
    price_max_cents: null,
    notify_email: true,
    notify_in_app: true,
    notify_push: false,
    match_count: 5,
    last_matched_at: null,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('FilterCard', () => {
  const mockOnEdit = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockUpdateIsPending.mockReturnValue(false)
    mockDeleteIsPending.mockReturnValue(false)
  })

  // ── Rendering ──

  it('renders filter name', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter()} onEdit={mockOnEdit} />
    )

    expect(screen.getByText('PHX punk shows')).toBeInTheDocument()
  })

  it('renders match count (plural)', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter({ match_count: 5 })} onEdit={mockOnEdit} />
    )

    expect(screen.getByText('5 matches')).toBeInTheDocument()
  })

  it('renders match count (singular)', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter({ match_count: 1 })} onEdit={mockOnEdit} />
    )

    expect(screen.getByText('1 match')).toBeInTheDocument()
  })

  it('renders filter summary with artist criteria', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter({ artist_ids: [1, 2] })} onEdit={mockOnEdit} />
    )

    expect(screen.getByText('2 artists')).toBeInTheDocument()
  })

  it('renders filter summary with venue criteria', () => {
    renderWithProviders(
      <FilterCard
        filter={makeFilter({ artist_ids: null, venue_ids: [1] })}
        onEdit={mockOnEdit}
      />
    )

    expect(screen.getByText('1 venue')).toBeInTheDocument()
  })

  it('renders filter summary with price criteria', () => {
    renderWithProviders(
      <FilterCard
        filter={makeFilter({ artist_ids: null, price_max_cents: 2500 })}
        onEdit={mockOnEdit}
      />
    )

    expect(screen.getByText('max $25')).toBeInTheDocument()
  })

  it('renders "free only" for price_max_cents=0', () => {
    renderWithProviders(
      <FilterCard
        filter={makeFilter({ artist_ids: null, price_max_cents: 0 })}
        onEdit={mockOnEdit}
      />
    )

    expect(screen.getByText('free only')).toBeInTheDocument()
  })

  it('renders last matched time when available', () => {
    // Use a date that is clearly in the past
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString()
    renderWithProviders(
      <FilterCard
        filter={makeFilter({ last_matched_at: twoHoursAgo })}
        onEdit={mockOnEdit}
      />
    )

    expect(screen.getByText(/Last:/)).toBeInTheDocument()
  })

  it('does not render last matched time when null', () => {
    renderWithProviders(
      <FilterCard
        filter={makeFilter({ last_matched_at: null })}
        onEdit={mockOnEdit}
      />
    )

    expect(screen.queryByText(/Last:/)).not.toBeInTheDocument()
  })

  // ── Active toggle ──

  it('renders switch with active state', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter({ is_active: true })} onEdit={mockOnEdit} />
    )

    const toggle = screen.getByRole('switch', { name: 'Pause filter' })
    expect(toggle).toBeInTheDocument()
    expect(toggle).toHaveAttribute('data-state', 'checked')
  })

  it('renders switch with inactive state', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter({ is_active: false })} onEdit={mockOnEdit} />
    )

    const toggle = screen.getByRole('switch', { name: 'Activate filter' })
    expect(toggle).toBeInTheDocument()
    expect(toggle).toHaveAttribute('data-state', 'unchecked')
  })

  it('calls updateFilter.mutate to toggle active state', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterCard filter={makeFilter({ id: 7, is_active: true })} onEdit={mockOnEdit} />
    )

    await user.click(screen.getByRole('switch', { name: 'Pause filter' }))

    expect(mockUpdateMutate).toHaveBeenCalledWith({
      id: 7,
      is_active: false,
    })
  })

  it('calls updateFilter.mutate to activate an inactive filter', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterCard filter={makeFilter({ id: 7, is_active: false })} onEdit={mockOnEdit} />
    )

    await user.click(screen.getByRole('switch', { name: 'Activate filter' }))

    expect(mockUpdateMutate).toHaveBeenCalledWith({
      id: 7,
      is_active: true,
    })
  })

  // ── Edit button ──

  it('calls onEdit when edit button is clicked', async () => {
    const user = userEvent.setup()
    const filter = makeFilter()
    renderWithProviders(
      <FilterCard filter={filter} onEdit={mockOnEdit} />
    )

    await user.click(screen.getByTitle('Edit filter'))

    expect(mockOnEdit).toHaveBeenCalledWith(filter)
  })

  // ── Delete flow ──

  it('does not show delete confirmation initially', () => {
    renderWithProviders(
      <FilterCard filter={makeFilter()} onEdit={mockOnEdit} />
    )

    expect(
      screen.queryByText('Delete this filter? This cannot be undone.')
    ).not.toBeInTheDocument()
  })

  it('shows delete confirmation after clicking Delete in dropdown', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterCard filter={makeFilter()} onEdit={mockOnEdit} />
    )

    // Open dropdown menu (the "more" button)
    const moreButtons = screen.getAllByRole('button')
    // The dropdown trigger is the last button in the actions area
    const dropdownTrigger = moreButtons.find(
      b => b.querySelector('svg') && !b.getAttribute('title')
    )
    if (dropdownTrigger) {
      await user.click(dropdownTrigger)
    }

    // Click Delete in dropdown
    const deleteMenuItem = await screen.findByText('Delete')
    await user.click(deleteMenuItem)

    expect(
      screen.getByText('Delete this filter? This cannot be undone.')
    ).toBeInTheDocument()
  })

  it('calls deleteFilter.mutate when confirming delete', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterCard filter={makeFilter({ id: 42 })} onEdit={mockOnEdit} />
    )

    // Open dropdown
    const moreButtons = screen.getAllByRole('button')
    const dropdownTrigger = moreButtons.find(
      b => b.querySelector('svg') && !b.getAttribute('title')
    )
    if (dropdownTrigger) {
      await user.click(dropdownTrigger)
    }

    // Click Delete in dropdown
    const deleteMenuItem = await screen.findByText('Delete')
    await user.click(deleteMenuItem)

    // Confirm delete
    const confirmDeleteButtons = screen.getAllByRole('button', { name: 'Delete' })
    // The confirmation Delete button is in the confirmation bar
    const confirmButton = confirmDeleteButtons.find(
      b => b.closest('.mt-3')
    )
    if (confirmButton) {
      await user.click(confirmButton)
    }

    expect(mockDeleteMutate).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('hides delete confirmation when Cancel is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterCard filter={makeFilter()} onEdit={mockOnEdit} />
    )

    // Open dropdown and click Delete
    const moreButtons = screen.getAllByRole('button')
    const dropdownTrigger = moreButtons.find(
      b => b.querySelector('svg') && !b.getAttribute('title')
    )
    if (dropdownTrigger) {
      await user.click(dropdownTrigger)
    }
    const deleteMenuItem = await screen.findByText('Delete')
    await user.click(deleteMenuItem)

    // Cancel
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(
      screen.queryByText('Delete this filter? This cannot be undone.')
    ).not.toBeInTheDocument()
  })

  // ── Disabled state during mutations ──

  it('disables toggle when mutation is pending', () => {
    mockUpdateIsPending.mockReturnValue(true)

    renderWithProviders(
      <FilterCard filter={makeFilter()} onEdit={mockOnEdit} />
    )

    const toggle = screen.getByRole('switch')
    expect(toggle).toBeDisabled()
  })
})
