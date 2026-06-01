import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { FestivalDetail, FestivalListItem } from '../types'

// Read hooks
const mockUseFestivals = vi.fn()
const mockUseFestival = vi.fn()
const mockUseFestivalLineup = vi.fn()
const mockUseFestivalVenues = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useFestivals: (opts: unknown) => mockUseFestivals(opts),
  useFestival: (opts: unknown) => mockUseFestival(opts),
  useFestivalLineup: (opts: unknown) => mockUseFestivalLineup(opts),
  useFestivalVenues: (opts: unknown) => mockUseFestivalVenues(opts),
}))

// Admin mutation hooks. The factory is hoisted above module scope by vitest,
// so the no-op return value is inlined rather than referencing an outer const.
vi.mock('../hooks/useAdminFestivals', () => {
  const noopMutation = () => ({ mutate: vi.fn(), isPending: false })
  return {
    useCreateFestival: noopMutation,
    useUpdateFestival: noopMutation,
    useDeleteFestival: noopMutation,
    useAddFestivalArtist: noopMutation,
    useUpdateFestivalArtist: noopMutation,
    useRemoveFestivalArtist: noopMutation,
    useAddFestivalVenue: noopMutation,
    useRemoveFestivalVenue: noopMutation,
  }
})

// Search hooks pulled in by the lineup/venue management panels
vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({ data: { artists: [] as unknown[] }, isLoading: false }),
}))
vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: { venues: [] as unknown[] }, isLoading: false }),
}))

import {
  FestivalManagement,
  CreateFestivalForm,
  EditFestivalFormFields,
} from './FestivalManagement'

function makeFestival(overrides: Partial<FestivalListItem> = {}): FestivalListItem {
  return {
    id: 1,
    name: 'FORM Arcosanti',
    slug: 'form-arcosanti-2025',
    series_slug: 'form-arcosanti',
    edition_year: 2025,
    city: 'Mayer',
    state: 'AZ',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    status: 'confirmed',
    artist_count: 12,
    venue_count: 1,
    ...overrides,
  }
}

function makeFestivalDetail(
  overrides: Partial<FestivalDetail> = {}
): FestivalDetail {
  return {
    id: 1,
    name: 'FORM Arcosanti',
    slug: 'form-arcosanti-2025',
    series_slug: 'form-arcosanti',
    edition_year: 2025,
    description: 'A music + arts festival.',
    location_name: 'Arcosanti',
    city: 'Mayer',
    state: 'AZ',
    country: 'US',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    website: 'https://experienceform.com',
    ticket_url: null,
    flyer_url: null,
    status: 'confirmed',
    social: null,
    artist_count: 0,
    venue_count: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('FestivalManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseFestival.mockReturnValue({ data: undefined, isLoading: false })
    mockUseFestivalLineup.mockReturnValue({
      data: { artists: [], count: 0 },
      isLoading: false,
    })
    mockUseFestivalVenues.mockReturnValue({
      data: { venues: [], count: 0 },
      isLoading: false,
    })
  })

  it('shows the loading spinner while festivals are fetching', () => {
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    // Loading state uses an inline Loader2 icon (not the shared LoadingSpinner
    // primitive), so the role="status" gain doesn't reach this site. Target
    // the animation class, which is the stable hook for inline spinners.
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders an error banner when the query fails', () => {
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('load failed'),
    })
    renderWithProviders(<FestivalManagement />)
    expect(screen.getByText('load failed')).toBeInTheDocument()
  })

  it('renders the status filter as the DS Select with the All sentinel', () => {
    // PSY-924: native <select> filter is now a Radix combobox; "All Statuses"
    // is the FILTER_SELECT_ALL sentinel round-tripped to '' for the query.
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    const statusSelect = screen.getByRole('combobox', { name: 'Filter by status' })
    expect(statusSelect).toHaveTextContent('All Statuses')
  })

  it('passes the selected status filter through to useFestivals', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('combobox', { name: 'Filter by status' }))
    await user.click(await screen.findByRole('option', { name: 'Cancelled' }))

    expect(mockUseFestivals).toHaveBeenLastCalledWith({ status: 'cancelled' })
  })

  it('round-trips the All sentinel back to no status filter', async () => {
    // Guards the sentinel: "All Statuses" must clear status ('' → undefined),
    // not pass the literal 'all' to the backend query.
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('combobox', { name: 'Filter by status' }))
    await user.click(await screen.findByRole('option', { name: 'Cancelled' }))
    expect(mockUseFestivals).toHaveBeenLastCalledWith({ status: 'cancelled' })

    await user.click(screen.getByRole('combobox', { name: 'Filter by status' }))
    await user.click(await screen.findByRole('option', { name: 'All Statuses' }))
    expect(mockUseFestivals).toHaveBeenLastCalledWith({ status: undefined })
  })

  it('renders the empty state when there are no festivals', () => {
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    expect(screen.getByText('No Festivals Found')).toBeInTheDocument()
    expect(
      screen.getByText(/No festivals yet\. Create your first festival/)
    ).toBeInTheDocument()
  })

  it('lists festivals with their status and a count', () => {
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [
          makeFestival({ id: 1, name: 'FORM Arcosanti' }),
          makeFestival({ id: 2, name: 'Desert Daze', status: 'announced' }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    expect(screen.getByText('FORM Arcosanti')).toBeInTheDocument()
    expect(screen.getByText('Desert Daze')).toBeInTheDocument()
    expect(screen.getByText('2 festivals')).toBeInTheDocument()
    // "Confirmed"/"Announced" also appear as <option> text in the status
    // filter; the row status badges are <div>s, so filter to those.
    const confirmedBadges = screen
      .getAllByText('Confirmed')
      .filter((el) => el.tagName === 'DIV')
    expect(confirmedBadges).toHaveLength(1)
    const announcedBadges = screen
      .getAllByText('Announced')
      .filter((el) => el.tagName === 'DIV')
    expect(announcedBadges).toHaveLength(1)
  })

  it('filters the list client-side by the debounced search input', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [
          makeFestival({ id: 1, name: 'FORM Arcosanti' }),
          makeFestival({ id: 2, name: 'Desert Daze' }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.type(screen.getByPlaceholderText('Search festivals...'), 'desert')

    await waitFor(() =>
      expect(screen.queryByText('FORM Arcosanti')).not.toBeInTheDocument()
    )
    expect(screen.getByText('Desert Daze')).toBeInTheDocument()
    expect(screen.getByText(/matching "desert"/)).toBeInTheDocument()
  })

  it('opens the create dialog from the New Festival button', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /New Festival/ }))
    expect(
      screen.getByRole('heading', { name: 'Create Festival' })
    ).toBeInTheDocument()
  })

  it('opens the delete confirmation with the festival name', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    // Edit/Delete are icon-only but each row buttons carry per-festival
    // aria-labels so they're addressable by accessible name.
    await user.click(
      screen.getByRole('button', { name: /delete FORM Arcosanti/i })
    )

    expect(
      screen.getByRole('heading', { name: 'Delete Festival' })
    ).toBeInTheDocument()
    // The confirmation copy echoes the name (quoted) in a bold span. The row
    // behind the dialog also shows the name, so match the quoted variant.
    expect(screen.getByText('"FORM Arcosanti"')).toBeInTheDocument()
  })

  it('exposes per-row Edit/Delete buttons with accessible names', () => {
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)
    // Positive assertion: per-row icon-only buttons must be reachable by
    // accessible name so keyboard + screen reader users can target them.
    expect(
      screen.getByRole('button', { name: /edit FORM Arcosanti/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /delete FORM Arcosanti/i })
    ).toBeInTheDocument()
  })

  it('disambiguates same-name festivals across editions in aria-labels', () => {
    // Festival names are not unique across editions (e.g. Coachella 2025 +
    // Coachella 2026). Per-row aria-labels must include edition_year so
    // screen reader users (and getByRole {name}) can target a specific row.
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [
          makeFestival({ id: 1, name: 'Coachella', edition_year: 2025 }),
          makeFestival({ id: 2, name: 'Coachella', edition_year: 2026 }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    expect(
      screen.getByRole('button', { name: 'Edit Coachella 2025' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Edit Coachella 2026' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Delete Coachella 2025' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Delete Coachella 2026' })
    ).toBeInTheDocument()
  })

  it('navigates into the lineup management panel', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ id: 7, name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /Lineup/ }))

    expect(
      screen.getByRole('heading', { name: /FORM Arcosanti - Lineup/ })
    ).toBeInTheDocument()
    expect(mockUseFestivalLineup).toHaveBeenCalledWith({
      festivalId: 7,
      enabled: true,
    })
    // Back button returns to the list
    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(
      screen.getByRole('heading', { name: 'Festivals' })
    ).toBeInTheDocument()
  })

  it('navigates into the venue management panel', async () => {
    const user = userEvent.setup()
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival({ id: 7, name: 'FORM Arcosanti' })], count: 1 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalManagement />)

    await user.click(screen.getByRole('button', { name: /Venues/ }))

    expect(
      screen.getByRole('heading', { name: /FORM Arcosanti - Venues/ })
    ).toBeInTheDocument()
    expect(mockUseFestivalVenues).toHaveBeenCalledWith({
      festivalIdOrSlug: 7,
      enabled: true,
    })
  })

  describe('EditFestivalFormFields: festival switch resets fields via key prop', () => {
    // Pins PSY-768: the inner form initializes local state from the festival
    // prop on mount, with no useEffect and no `initialized` ratchet. Callers
    // pass `key={festival.id}` so React unmounts + remounts with fresh state
    // when the festival switches. The two assertions below are the
    // load-bearing pair — without both, a future maintainer could re-add a
    // festival-prop-based reset and the tests would still pass.

    it('resets fields when re-rendered with a different festival (via key prop)', async () => {
      const user = userEvent.setup()
      const festivalA = makeFestivalDetail({
        id: 1,
        name: 'FORM Arcosanti',
        city: 'Mayer',
        edition_year: 2025,
      })
      const festivalB = makeFestivalDetail({
        id: 2,
        name: 'M3F',
        city: 'Phoenix',
        edition_year: 2026,
      })

      const { rerender } = renderWithProviders(
        <EditFestivalFormFields
          key={festivalA.id}
          festival={festivalA}
          open
          onOpenChange={vi.fn()}
          onSuccess={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText('Name *')
      expect(nameInput).toHaveValue('FORM Arcosanti')

      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')
      expect(nameInput).toHaveValue('Dirty Edit')

      rerender(
        <EditFestivalFormFields
          key={festivalB.id}
          festival={festivalB}
          open
          onOpenChange={vi.fn()}
          onSuccess={vi.fn()}
        />
      )

      expect(screen.getByLabelText('Name *')).toHaveValue('M3F')
      expect(screen.getByLabelText('City')).toHaveValue('Phoenix')
      expect(screen.getByLabelText('Edition Year')).toHaveValue(2026)
    })

    it('preserves dirty edits when re-rendered with the same key', async () => {
      const user = userEvent.setup()
      const festival = makeFestivalDetail({ id: 1, name: 'FORM Arcosanti' })

      const { rerender } = renderWithProviders(
        <EditFestivalFormFields
          key={festival.id}
          festival={festival}
          open
          onOpenChange={vi.fn()}
          onSuccess={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText('Name *')
      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')

      rerender(
        <EditFestivalFormFields
          key={festival.id}
          festival={festival}
          open
          onOpenChange={vi.fn()}
          onSuccess={vi.fn()}
        />
      )

      expect(screen.getByLabelText('Name *')).toHaveValue('Dirty Edit')
    })
  })

  // Pins the PSY-930 Dialog->Sheet migration: AdminFormLayout keeps the create
  // form mounted across the Sheet close animation, so the form must clear its
  // own state when it (re)opens. Mirrors CreateStationForm's reset-on-open test.
  describe('CreateFestivalForm reset-on-open (PSY-930)', () => {
    it('clears entered field values when the Sheet is closed and reopened', async () => {
      const user = userEvent.setup()
      const { rerender } = renderWithProviders(
        <CreateFestivalForm open={false} onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      )

      rerender(
        <CreateFestivalForm open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      )
      const nameInput = screen.getByLabelText('Name *')
      await user.type(nameInput, 'M3F Festival')
      expect(nameInput).toHaveValue('M3F Festival')

      rerender(
        <CreateFestivalForm open={false} onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      )
      rerender(
        <CreateFestivalForm open onOpenChange={vi.fn()} onSuccess={vi.fn()} />
      )
      expect(screen.getByLabelText('Name *')).toHaveValue('')
    })
  })
})
