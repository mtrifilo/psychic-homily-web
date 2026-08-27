import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationFilter } from '../types'

// ── Mocks ──────────────────────────────────────────

const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()

vi.mock('../hooks', () => ({
  useCreateFilter: () => ({
    mutate: mockCreateMutate,
    isPending: false,
  }),
  useUpdateFilter: () => ({
    mutate: mockUpdateMutate,
    isPending: false,
  }),
}))

// Mock search hooks used by MultiSelectSearch
vi.mock('@/features/artists/hooks/useArtistSearch', () => ({
  useArtistSearch: () => ({
    data: { artists: [] as unknown[] },
    isLoading: false,
  }),
}))

vi.mock('@/features/venues/hooks/useVenueSearch', () => ({
  useVenueSearch: () => ({
    data: { venues: [] as unknown[] },
    isLoading: false,
  }),
}))

vi.mock('@/features/tags/hooks', () => ({
  useSearchTags: () => ({
    data: { tags: [] as unknown[] },
    isLoading: false,
  }),
}))

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    LABELS: { LIST: '/labels', GET: (id: number) => `/labels/${id}` },
    ARTISTS: { GET: (id: number) => `/artists/${id}` },
    VENUES: { GET: (id: number) => `/venues/${id}` },
    TAGS: { GET: (id: number) => `/tags/${id}` },
  },
  API_BASE_URL: '',
}))

import { FilterForm } from './FilterForm'

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
    source: 'user',
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

describe('FilterForm', () => {
  const mockOnOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  // ── Create mode ──

  it('renders "New custom alert" title in create mode', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(screen.getByText('New custom alert')).toBeInTheDocument()
  })

  it('renders description text in create mode', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(
      screen.getByText(/Build an alert to be told when matching shows are added/)
    ).toBeInTheDocument()
  })

  it('renders "Create Filter" submit button in create mode', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(
      screen.getByRole('button', { name: 'Create alert' })
    ).toBeInTheDocument()
  })

  it('renders Cancel button', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(mockOnOpenChange).toHaveBeenCalledWith(false)
  })

  // ── Edit mode ──

  it('renders "Edit custom alert" title in edit mode', () => {
    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={makeFilter()}
      />
    )

    expect(
      screen.getByText('Edit custom alert')
    ).toBeInTheDocument()
  })

  it('renders "Save Changes" submit button in edit mode', () => {
    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={makeFilter()}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Save Changes' })
    ).toBeInTheDocument()
  })

  it('pre-populates name field when editing', () => {
    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={makeFilter({ name: 'My Filter' })}
      />
    )

    const nameInput = screen.getByLabelText('Alert name') as HTMLInputElement
    expect(nameInput.value).toBe('My Filter')
  })

  it('hydrates entity chips in edit mode when IDs are resolved', async () => {
    // Mock apiRequest to resolve entity IDs to names
    mockApiRequest.mockImplementation((url: string) => {
      if (url === '/artists/1') return Promise.resolve({ id: 1, name: 'Artist One' })
      if (url === '/artists/2') return Promise.resolve({ id: 2, name: 'Artist Two' })
      if (url === '/venues/5') return Promise.resolve({ id: 5, name: 'The Venue' })
      return Promise.resolve({})
    })

    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={makeFilter({
          artist_ids: [1, 2],
          venue_ids: [5],
        })}
      />
    )

    // Wait for entity names to be resolved and displayed as chips
    await waitFor(() => {
      expect(screen.getByText('Artist One')).toBeInTheDocument()
    })
    expect(screen.getByText('Artist Two')).toBeInTheDocument()
    expect(screen.getByText('The Venue')).toBeInTheDocument()
  })

  it('pre-populates price field when editing', () => {
    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={makeFilter({ price_max_cents: 2500 })}
      />
    )

    const priceInput = screen.getByLabelText('Max Price') as HTMLInputElement
    expect(priceInput.value).toBe('25')
  })

  // ── Form fields ──

  it('renders all form sections', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(screen.getByLabelText('Alert name')).toBeInTheDocument()
    expect(screen.getByText('Artists')).toBeInTheDocument()
    expect(screen.getByText('Venues')).toBeInTheDocument()
    expect(screen.getByText('Labels')).toBeInTheDocument()
    expect(screen.getByText('Tags (match any)')).toBeInTheDocument()
    expect(screen.getByText('Exclude Tags')).toBeInTheDocument()
    expect(screen.getByLabelText('Max Price')).toBeInTheDocument()
    expect(screen.getByText('Notify via')).toBeInTheDocument()
    expect(screen.getByText('Email')).toBeInTheDocument()
  })

  // ── Validation ──

  it('disables submit when name is empty', () => {
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    expect(
      screen.getByRole('button', { name: 'Create alert' })
    ).toBeDisabled()
  })

  it('disables submit when name is present but no criteria', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    const nameInput = screen.getByLabelText('Alert name')
    await user.type(nameInput, 'My Filter')

    expect(
      screen.getByRole('button', { name: 'Create alert' })
    ).toBeDisabled()
  })

  it('shows criteria warning when name is present but no criteria selected', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    const nameInput = screen.getByLabelText('Alert name')
    await user.type(nameInput, 'My Filter')

    expect(
      screen.getByText(/Add at least one criteria/)
    ).toBeInTheDocument()
  })

  it('enables submit when name and price criteria are provided', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    await user.type(screen.getByLabelText('Alert name'), 'Free Shows')
    await user.type(screen.getByLabelText('Max Price'), '0')

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Create alert' })
      ).not.toBeDisabled()
    })
  })

  // ── Submit ──

  it('calls createFilter.mutate on submit in create mode', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    await user.type(screen.getByLabelText('Alert name'), 'Free Shows')
    await user.type(screen.getByLabelText('Max Price'), '0')

    await user.click(screen.getByRole('button', { name: 'Create alert' }))

    expect(mockCreateMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Free Shows',
        price_max_cents: 0,
        notify_email: true,
        notify_in_app: true,
      }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('calls updateFilter.mutate on submit in edit mode', async () => {
    const user = userEvent.setup()
    const filter = makeFilter({ id: 7, name: 'Old Name', price_max_cents: 2500 })
    renderWithProviders(
      <FilterForm
        open={true}
        onOpenChange={mockOnOpenChange}
        filter={filter}
      />
    )

    // Change the name
    const nameInput = screen.getByLabelText('Alert name') as HTMLInputElement
    await user.clear(nameInput)
    await user.type(nameInput, 'New Name')

    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mockUpdateMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 7,
        name: 'New Name',
      }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  // ── Not rendered when closed ──

  it('does not render content when open is false', () => {
    renderWithProviders(
      <FilterForm open={false} onOpenChange={mockOnOpenChange} />
    )

    expect(
      screen.queryByText('New custom alert')
    ).not.toBeInTheDocument()
  })

  // ── Notification channels ──

  it('toggles email notification switch', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <FilterForm open={true} onOpenChange={mockOnOpenChange} />
    )

    // The email switch should be checked by default
    const switches = screen.getAllByRole('switch')
    // Find the Email switch - it's the first one in the "Notify via" section
    const emailSwitch = switches.find(s => {
      const label = s.closest('.flex')?.querySelector('span')
      return label?.textContent === 'Email'
    })
    expect(emailSwitch).toBeTruthy()
  })

  // MultiSelectSearch's blur delay used to be an untracked `setTimeout`, so it
  // still fired ~200ms after unmount and called `setState` into a torn-down
  // React DOM. Under vitest that lands after jsdom teardown and throws
  // `ReferenceError: window is not defined`, failing the whole run.
  it('leaves no pending blur timer behind on unmount', () => {
    vi.useFakeTimers()
    try {
      const { unmount } = renderWithProviders(
        <FilterForm open={true} onOpenChange={mockOnOpenChange} />
      )
      // Baseline-delta rather than an absolute 0: react-query and Radix keep
      // their own timers alive past unmount, which this test is not about.
      const baseline = vi.getTimerCount()
      fireEvent.blur(screen.getByPlaceholderText('Search artists...'))
      expect(vi.getTimerCount()).toBeGreaterThan(baseline)

      unmount()
      expect(vi.getTimerCount()).toBeLessThanOrEqual(baseline)
    } finally {
      vi.useRealTimers()
    }
  })
})
