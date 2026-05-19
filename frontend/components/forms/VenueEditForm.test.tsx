import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { VenueEditForm } from './VenueEditForm'
import type { VenueWithShowCount } from '@/features/venues'

// --- Mocks ---

const mockAuthContext = vi.fn(() => ({
  user: { id: 1, is_admin: true } as { id: number; is_admin: boolean } | null,
  isAuthenticated: true,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// `useVenueUpdate` is re-exported from `@/features/venues`. Mock that
// surface (matches how VenueEditForm imports it).
vi.mock('@/features/venues', async () => {
  const actual = await vi.importActual<typeof import('@/features/venues')>(
    '@/features/venues'
  )
  return {
    ...actual,
    useVenueUpdate: () => ({ mutate: vi.fn(), isPending: false }),
  }
})

// --- Helpers ---

function makeVenue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'venue-a',
    name: 'Venue A',
    address: '123 Main St',
    city: 'Phoenix',
    state: 'AZ',
    zipcode: '85001',
    verified: true,
    upcoming_show_count: 3,
    social: {
      instagram: 'https://instagram.com/venue-a',
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: 'https://venue-a.com',
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('VenueEditForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('initial render', () => {
    it('populates fields from the venue prop', () => {
      const venue = makeVenue()
      renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Venue A')
      expect(screen.getByLabelText(/Address/i)).toHaveValue('123 Main St')
      expect(screen.getByLabelText(/City/i)).toHaveValue('Phoenix')
      expect(screen.getByLabelText(/State/i)).toHaveValue('AZ')
      expect(screen.getByLabelText(/Zipcode/i)).toHaveValue('85001')
    })

    it('returns null for non-admin users', () => {
      mockAuthContext.mockReturnValueOnce({
        user: { id: 1, is_admin: false } as { id: number; is_admin: boolean } | null,
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })

      const venue = makeVenue()
      const { container } = renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      expect(container).toBeEmptyDOMElement()
    })
  })

  describe('venue switch with key prop (regression: PSY-732)', () => {
    // Contract: parent components MUST pass `key={venue.id}` so React
    // unmounts + remounts the form with fresh state when the venue
    // switches. This replaces a `useEffect` reset that is the
    // anti-pattern per feedback_no_useeffect_for_prop_derived_state.md.
    it('resets fields when re-rendered with a different venue (via key prop)', async () => {
      const user = userEvent.setup()
      const venueA = makeVenue({
        id: 1,
        name: 'Venue A',
        city: 'Phoenix',
        state: 'AZ',
      })
      const venueB = makeVenue({
        id: 2,
        name: 'Venue B',
        city: 'Tucson',
        state: 'AZ',
        address: '456 Oak Ave',
        zipcode: '85701',
        social: {
          instagram: 'https://instagram.com/venue-b',
          facebook: null,
          twitter: null,
          youtube: null,
          spotify: null,
          soundcloud: null,
          bandcamp: null,
          website: 'https://venue-b.com',
        },
      })

      const { rerender } = renderWithProviders(
        <VenueEditForm
          key={venueA.id}
          venue={venueA}
          open
          onOpenChange={vi.fn()}
        />
      )

      // Sanity: venue A's data is loaded.
      const nameInput = screen.getByLabelText(/Venue Name/i)
      expect(nameInput).toHaveValue('Venue A')

      // User edits a field but does NOT save.
      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')
      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Dirty Edit')

      // Parent swaps to venue B with the new key — React unmounts +
      // remounts the form. Fields must now show venue B's values, NOT
      // the dirty "Dirty Edit" text and NOT venue A's original values.
      rerender(
        <VenueEditForm
          key={venueB.id}
          venue={venueB}
          open
          onOpenChange={vi.fn()}
        />
      )

      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Venue B')
      expect(screen.getByLabelText(/City/i)).toHaveValue('Tucson')
      expect(screen.getByLabelText(/Address/i)).toHaveValue('456 Oak Ave')
      expect(screen.getByLabelText(/Zipcode/i)).toHaveValue('85701')
    })

    it('preserves dirty edits when re-rendered with same key + same venue', async () => {
      // Negative case: if the parent re-renders with the SAME key (e.g.,
      // an unrelated state update), the form must NOT reset. This
      // demonstrates the `key` is the load-bearing mechanism for the
      // reset — not the venue prop alone.
      const user = userEvent.setup()
      const venue = makeVenue({ id: 1, name: 'Venue A' })

      const { rerender } = renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText(/Venue Name/i)
      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')

      rerender(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      // Same key → same instance → dirty edit survives.
      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Dirty Edit')
    })
  })
})
