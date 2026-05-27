import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
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
const mockMutate = vi.fn()
vi.mock('@/features/venues', async () => {
  const actual = await vi.importActual<typeof import('@/features/venues')>(
    '@/features/venues'
  )
  return {
    ...actual,
    useVenueUpdate: () => ({ mutate: mockMutate, isPending: false }),
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
    mockMutate.mockReset()
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

  describe('venue switch resets fields via key prop', () => {
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

      const nameInput = screen.getByLabelText(/Venue Name/i)
      expect(nameInput).toHaveValue('Venue A')

      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')
      expect(nameInput).toHaveValue('Dirty Edit')

      rerender(
        <VenueEditForm
          key={venueB.id}
          venue={venueB}
          open
          onOpenChange={vi.fn()}
        />
      )

      // Re-query after rerender — the key change unmounts the previous
      // input node, so `nameInput` no longer points at a live element.
      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Venue B')
      expect(screen.getByLabelText(/City/i)).toHaveValue('Tucson')
      expect(screen.getByLabelText(/Address/i)).toHaveValue('456 Oak Ave')
      expect(screen.getByLabelText(/Zipcode/i)).toHaveValue('85701')
    })

    it('preserves dirty edits when re-rendered with the same key', async () => {
      // Pins the `key` as the load-bearing reset mechanism: if React
      // re-renders the same instance (no key change), the dirty edit
      // must survive. Without this, a future maintainer could
      // accidentally add a venue-prop-based reset and have both tests
      // still pass.
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

      expect(screen.getByLabelText(/Venue Name/i)).toHaveValue('Dirty Edit')
    })
  })

  describe('inline validation messages', () => {
    // Mirrors the PSY-779 fix on ArtistEditForm: the zod `onSubmit` validator
    // rejects empty required fields, but the form must ALSO surface the
    // message inline (via FieldInfo) and set aria-invalid on the input — or
    // the user is left with a silent, dead Save button.
    it('surfaces "Venue name is required" and blocks the mutation when the required name is cleared, then clears on valid input', async () => {
      const user = userEvent.setup()
      const venue = makeVenue()
      renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText(/Venue Name/i)
      await user.clear(nameInput)
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(
        await screen.findByText(/Venue name is required/i)
      ).toBeInTheDocument()
      expect(nameInput).toHaveAttribute('aria-invalid', 'true')
      expect(mockMutate).not.toHaveBeenCalled()

      // Typing a valid value clears the error + aria-invalid, proving the
      // message is reactive to field state (avoids the PSY-859 false-coverage
      // pattern where a test only asserts the failing branch).
      await user.type(nameInput, 'Venue C')

      await waitFor(() => {
        expect(
          screen.queryByText(/Venue name is required/i)
        ).not.toBeInTheDocument()
      })
      expect(nameInput).toHaveAttribute('aria-invalid', 'false')
    })

    it('surfaces "City is required" and blocks the mutation when the required city is cleared, then clears on valid input', async () => {
      const user = userEvent.setup()
      const venue = makeVenue()
      renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      const cityInput = screen.getByLabelText(/^City \*/i)
      await user.clear(cityInput)
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(
        await screen.findByText(/City is required/i)
      ).toBeInTheDocument()
      expect(cityInput).toHaveAttribute('aria-invalid', 'true')
      expect(mockMutate).not.toHaveBeenCalled()

      await user.type(cityInput, 'Tucson')

      await waitFor(() => {
        expect(screen.queryByText(/City is required/i)).not.toBeInTheDocument()
      })
      expect(cityInput).toHaveAttribute('aria-invalid', 'false')
    })

    it('surfaces "State is required" and blocks the mutation when the required state is cleared, then clears on valid input', async () => {
      const user = userEvent.setup()
      const venue = makeVenue()
      renderWithProviders(
        <VenueEditForm
          key={venue.id}
          venue={venue}
          open
          onOpenChange={vi.fn()}
        />
      )

      const stateInput = screen.getByLabelText(/^State \*/i)
      await user.clear(stateInput)
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(
        await screen.findByText(/State is required/i)
      ).toBeInTheDocument()
      expect(stateInput).toHaveAttribute('aria-invalid', 'true')
      expect(mockMutate).not.toHaveBeenCalled()

      await user.type(stateInput, 'AZ')

      await waitFor(() => {
        expect(screen.queryByText(/State is required/i)).not.toBeInTheDocument()
      })
      expect(stateInput).toHaveAttribute('aria-invalid', 'false')
    })
  })
})
