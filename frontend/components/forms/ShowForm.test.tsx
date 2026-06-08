import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ExtractedShowData } from '@/lib/types/extraction'
import type { ShowResponse } from '@/features/shows'

// ─────────────────────────────────────────────────────────────
// Shared mock state
//
// All mocks are configurable per-test via these module-scoped objects.
// Tests mutate the relevant fields in beforeEach / inside it() blocks
// BEFORE rendering ShowForm; the mocks read them at call time so each
// test gets the wiring it needs without duplicating the vi.mock setup.
// ─────────────────────────────────────────────────────────────

const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
}

const mockAuth = {
  user: { id: 1, is_admin: false } as { id: number; is_admin: boolean } | null,
  isAuthenticated: true,
  isLoading: false,
}

type MutateFn = (vars: unknown, opts?: {
  onSuccess?: (data: unknown) => void
  onError?: (err: Error) => void
}) => void

const mockShowSubmit = {
  mutate: vi.fn() as MutateFn & ReturnType<typeof vi.fn>,
  isPending: false,
  error: null as Error | null,
  reset: vi.fn(),
}

const mockShowUpdate = {
  mutate: vi.fn() as MutateFn & ReturnType<typeof vi.fn>,
  isPending: false,
  error: null as Error | null,
  reset: vi.fn(),
}

interface MockVenue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  verified: boolean
}

const mockVenueSearch: { venues: MockVenue[] } = { venues: [] }

vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuth,
}))

vi.mock('@/features/shows', () => ({
  useShowSubmit: () => mockShowSubmit,
  useShowUpdate: () => mockShowUpdate,
}))

// Return at least one search result so ArtistInput's dropdown can transition
// to aria-expanded="true". The component gates aria-expanded on
// `showDropdown && filteredArtists.length > 0`, so an empty result would
// suppress the canary state the PSY-724 test relies on.
vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({
    data: { artists: [{ id: 999, name: 'Match', city: 'Phoenix', state: 'AZ' }] },
    isLoading: false,
  }),
  getArtistLocation: () => '',
}))

vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: { venues: mockVenueSearch.venues }, isLoading: false }),
  getVenueLocation: () => '',
}))

// Imported AFTER mocks so the component picks up the stubbed modules.
import { ShowForm } from './ShowForm'

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

/**
 * Reset shared mock state between tests. Each test starts from the
 * canonical "non-admin, no pending mutations, empty venue search" baseline.
 * vi.clearAllMocks() resets the spy call history; this resets the data
 * the spies read at call time.
 */
function resetMockState() {
  mockAuth.user = { id: 1, is_admin: false }
  mockAuth.isAuthenticated = true
  mockShowSubmit.isPending = false
  mockShowSubmit.error = null
  mockShowSubmit.mutate.mockReset()
  mockShowUpdate.isPending = false
  mockShowUpdate.error = null
  mockShowUpdate.mutate.mockReset()
  mockVenueSearch.venues = []
}

/**
 * Build a future date string (YYYY-MM-DD) one year out, to dodge the
 * "date cannot be in the past" zod refinement without hardcoding a
 * historical date that will rot under vi.useRealTimers() (the PSY-859
 * anti-pattern called out in the ticket).
 */
function futureDate(): string {
  const d = new Date()
  d.setFullYear(d.getFullYear() + 1)
  return d.toISOString().slice(0, 10) // YYYY-MM-DD
}

/**
 * Build a past date string (one year ago) for date-validation tests.
 * Same anti-rot reasoning as futureDate().
 */
function pastDate(): string {
  const d = new Date()
  d.setFullYear(d.getFullYear() - 1)
  return d.toISOString().slice(0, 10)
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 42,
    slug: 'edit-me',
    title: 'Edit Me Show',
    event_date: '2099-06-15T03:00:00Z', // far-future to skip past-date validator on re-submit
    city: 'Phoenix',
    state: 'AZ',
    price: 25,
    age_requirement: '21+',
    description: 'A pre-existing show.',
    image_url: 'https://example.com/flyer.jpg',
    ticket_url: null,
    status: 'approved',
    submitted_by: 1,
    rejection_reason: null,
    rejection_category: null,
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 5,
        slug: 'valley-bar',
        name: 'Valley Bar',
        address: '130 N Central Ave',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      {
        id: 11,
        slug: 'the-mountain-goats',
        name: 'The Mountain Goats',
        is_headliner: true,
        set_type: 'headliner',
        position: 1,
        socials: {},
      },
    ],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

// ─────────────────────────────────────────────────────────────
// Regression guards (PSY-724 stable keys)
//
// These were the original tests. They use the shared mock state defined
// above. Kept in their own describe blocks so the file's intent is clear
// at a scan: regressions first, behavioral coverage second.
// ─────────────────────────────────────────────────────────────

describe('ShowForm — artists list stable keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('removing the middle artist keeps each remaining row\'s component state with its row (not its index)', () => {
    // PSY-1006 flake fix: drive this test with fireEvent, NOT userEvent.
    // userEvent moves real focus between rows; ArtistInput.handleBlur schedules
    // setTimeout(close, 150ms) on blur, and that timer raced the synchronous
    // aria-expanded assertions below — under CI load the 150ms elapsed first,
    // the dropdown closed (aria-expanded → "false"), and the test failed
    // intermittently. fireEvent doesn't manage focus, so no blur fires, no
    // close timer is scheduled, and the open state stays deterministic. The
    // test only exercises onChange-driven open state + key stability, which
    // fireEvent covers fully.
    renderWithProviders(<ShowForm mode="create" />)

    // Start with 1, add two more → 3 artist rows.
    const addButton = screen.getByRole('button', { name: /add another artist/i })
    fireEvent.click(addButton)
    fireEvent.click(addButton)

    const getInputs = () =>
      screen.getAllByPlaceholderText('Enter artist name') as HTMLInputElement[]
    expect(getInputs()).toHaveLength(3)

    // Set rows 0 and 2 to distinct values (opens their dropdowns). Row 1 stays
    // empty so the bug — if present — surfaces as the third row's local
    // ArtistInput state (typed value + aria-expanded dropdown state) leaking
    // onto the new second slot after the middle row is removed.
    fireEvent.change(getInputs()[0], { target: { value: 'Artist A' } })
    fireEvent.change(getInputs()[2], { target: { value: 'Artist C' } })

    // ArtistInput.tsx opens its autocomplete listbox the moment the input has
    // any value (see isOpen / showDropdown). aria-expanded reflects the local
    // useState in that specific component instance, which is exactly what
    // leaks across rows when React reuses an instance via a stale key.
    expect(getInputs()[0]).toHaveAttribute('aria-expanded', 'true')
    expect(getInputs()[1]).toHaveAttribute('aria-expanded', 'false')
    expect(getInputs()[2]).toHaveAttribute('aria-expanded', 'true')

    // Remove the empty middle row.
    const removeButtons = screen.getAllByRole('button', { name: /remove artist/i })
    expect(removeButtons).toHaveLength(3)
    fireEvent.click(removeButtons[1])

    const remaining = getInputs()
    expect(remaining).toHaveLength(2)

    // The form value at each row index reflects the new array — both with
    // and without the bug, because that's controlled by TanStack form state,
    // not React keys.
    expect(remaining[0].value).toBe('Artist A')
    expect(remaining[1].value).toBe('Artist C')

    // The dropdown state is the canary: with key={index}, React would reuse
    // the second-row ArtistInput instance for the new second row (which is
    // semantically the old third row). The new second row would display
    // "Artist C" but its internal state (aria-expanded) would belong to the
    // removed empty row, surfacing as aria-expanded="false".
    expect(remaining[1]).toHaveAttribute('aria-expanded', 'true')
  })
})

// ─────────────────────────────────────────────────────────────
// PSY-693 coverage: behavioral surface
//
// The PSY-724 test above proves the stable-keys regression cannot return.
// The blocks below cover the load-bearing user flows that, if broken,
// would silently block show submissions:
//   - All required fields render in create mode
//   - Artists list add/remove updates the visible row count
//   - Venue select auto-fills city/state; verified venues lock the fields
//   - Past-date validation blocks submit + surfaces a message
//   - Successful submit calls useShowSubmit.mutate; onSuccess wires through
//   - AI extraction seeds the form via defaultValues at mount; a new
//     extraction re-seeds it on key-remount (PSY-795)
//   - Edit mode pre-fills from `initialData`
//   - The "do not publish" private toggle is create-only (hidden in edit)
// ─────────────────────────────────────────────────────────────

describe('ShowForm — required fields render in create mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('renders artist, venue, date, time, cost, ages, and description fields', () => {
    renderWithProviders(<ShowForm mode="create" />)

    // Artist field starts at one row (defaultFormValues).
    expect(screen.getByPlaceholderText('Enter artist name')).toBeInTheDocument()
    // Venue input (combobox role) is present.
    expect(screen.getByLabelText(/^Venue$/i)).toBeInTheDocument()
    // City + State live in the venue grid.
    expect(screen.getByLabelText(/^City$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^State$/i)).toBeInTheDocument()
    // Date + Time in the date grid.
    expect(screen.getByLabelText(/^Date$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Time$/i)).toBeInTheDocument()
    // Cost + Ages + Description in additional details.
    expect(screen.getByLabelText(/cost/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/ages/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument()
    // Submit button uses create-mode copy.
    expect(screen.getByRole('button', { name: /submit show/i })).toBeInTheDocument()
  })

  it('hides the image_url field in create mode (per PSY-521 — edit-only)', () => {
    renderWithProviders(<ShowForm mode="create" />)
    expect(screen.queryByLabelText(/image url/i)).not.toBeInTheDocument()
  })
})

describe('ShowForm — add / remove artist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('clicking "Add another artist" appends a row; clicking remove drops it', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(1)
    // First row has no remove button (showRemoveButton hides when length <= 1).
    expect(screen.queryByRole('button', { name: /remove artist/i })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /add another artist/i }))
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(2)
    // Now both rows expose a remove button.
    expect(screen.getAllByRole('button', { name: /remove artist/i })).toHaveLength(2)

    await user.click(screen.getByRole('button', { name: /add another artist/i }))
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(3)

    // Remove the middle row → back to 2.
    const removeButtons = screen.getAllByRole('button', { name: /remove artist/i })
    await user.click(removeButtons[1])
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(2)
  })
})

describe('ShowForm — venue selection auto-fill + verified-venue lock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('selecting an unverified venue from the dropdown auto-fills city/state and leaves them editable', async () => {
    mockVenueSearch.venues = [
      {
        id: 7,
        slug: 'unverified-spot',
        name: 'Unverified Spot',
        address: '99 Test Rd',
        city: 'Tucson',
        state: 'AZ',
        verified: false,
      },
    ]
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    const venueInput = screen.getByLabelText(/^Venue$/i)
    // Type to open the dropdown — VenueInput gates the dropdown on
    // searchValue.length > 0 (handleInputChange → setIsOpen(value.length > 0)).
    await user.type(venueInput, 'Unverified')

    // Click the existing-venue option. data-testid is set on VenueInput's
    // option button; this is the most stable selector for the dropdown items.
    const option = await screen.findByTestId('search-result-venue')
    // Use mouseDown because VenueInput.handleVenueSelect runs on onMouseDown
    // (so a click event isn't needed; mouse-down propagates synchronously).
    await user.pointer({ keys: '[MouseLeft>]', target: option })

    // city/state are populated, but remain editable for a non-admin selecting
    // an unverified venue (isVenueLocationEditable: !verified → true).
    const city = screen.getByLabelText(/^City$/i) as HTMLInputElement
    const state = screen.getByLabelText(/^State$/i) as HTMLInputElement
    await waitFor(() => expect(city.value).toBe('Tucson'))
    expect(state.value).toBe('AZ')
    expect(city).not.toBeDisabled()
    expect(state).not.toBeDisabled()
  })

  it('selecting a verified venue locks city/state/address for non-admins', async () => {
    mockVenueSearch.venues = [
      {
        id: 9,
        slug: 'verified-spot',
        name: 'Verified Spot',
        address: '1 Verified Way',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ]
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    await user.type(screen.getByLabelText(/^Venue$/i), 'Verified')
    const option = await screen.findByTestId('search-result-venue')
    await user.pointer({ keys: '[MouseLeft>]', target: option })

    const city = screen.getByLabelText(/^City$/i) as HTMLInputElement
    const state = screen.getByLabelText(/^State$/i) as HTMLInputElement
    const address = screen.getByLabelText(/^Address/i) as HTMLInputElement

    await waitFor(() => expect(city.value).toBe('Phoenix'))
    expect(state.value).toBe('AZ')
    // address from the venue is filled in too
    expect(address.value).toBe('1 Verified Way')

    // Verified venue → fields are locked for non-admin
    // (computeVenueEditable returns false when selectedVenue.verified is true
    // and the user is not admin).
    expect(city).toBeDisabled()
    expect(state).toBeDisabled()
    expect(address).toBeDisabled()

    // And the "Verified Venue" admin-info banner appears.
    expect(screen.getByText(/Verified Venue/i)).toBeInTheDocument()
  })
})

describe('ShowForm — date validation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('blocks submit and surfaces "Date cannot be in the past" when a past date is selected', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    // Fill the required fields with a past date.
    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Some Artist')
    await user.type(screen.getByLabelText(/^Venue$/i), 'A Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    // Date input — user.type is unreliable on HTML5 date inputs in jsdom,
    // so set the value via the native setter + input event (see fireSet).
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, pastDate())

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    // Zod refinement message bubbles into FieldInfo.
    expect(
      await screen.findByText(/Date cannot be in the past/i)
    ).toBeInTheDocument()
    // And the submit mutation was never invoked.
    expect(mockShowSubmit.mutate).not.toHaveBeenCalled()
  })
})

// Direct value-set helper — jsdom's HTML5 date input + user.type combination
// is fiddly enough that the explicit DOM mutation is the most stable path.
// React's onChange handler still fires via the input event dispatched here.
function fireSet(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    'value'
  )?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

describe('ShowForm — successful submit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('invokes useShowSubmit.mutate with the assembled submission and fires onSuccess', async () => {
    const onSuccess = vi.fn()
    // The form calls onSuccess after a 1500ms timeout, OR redirects via
    // router.push. We pass redirectOnCreate={false} so the onSuccess
    // path is the one we exercise.
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="create" onSuccess={onSuccess} redirectOnCreate={false} />
    )

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Headliner Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Some Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))

    const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
      artists: Array<{ name: string; is_headliner: boolean }>
      venues: Array<{ name: string; city: string; state: string }>
      city: string
      state: string
    }
    expect(submission.city).toBe('Phoenix')
    expect(submission.state).toBe('AZ')
    expect(submission.venues[0].name).toBe('Some Venue')
    expect(submission.artists[0].name).toBe('Headliner Band')
    // First artist defaults to is_headliner=true (defaultFormValues).
    expect(submission.artists[0].is_headliner).toBe(true)

    // Success branch fires onSuccess after 1500ms timeout.
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1), {
      timeout: 2500,
    })
  })
})

// PSY-795: AI extraction is now folded into TanStack Form's `defaultValues`
// at mount (calculate-during-render via mergeExtraction), replacing the old
// prop-derived useEffect + rAF + lastAppliedExtraction ref. The parent
// (app/shows/submit/page.tsx) remounts ShowForm via a `key` it bumps on each
// extraction; these tests model that contract:
//   1. An extraction passed at mount seeds every field.
//   2. A NEW extraction with a fresh `key` re-seeds the form (remount).
//   3. Without a key change, defaultValues are read once — a re-render that
//      keeps the same key does NOT clobber a user edit.
describe('ShowForm — AI extraction seeds defaultValues + key-remount re-seed', () => {
  const extraction: ExtractedShowData = {
    artists: [{ name: 'AI Artist', is_headliner: true }],
    venue: { name: 'AI Venue', city: 'Tempe', state: 'AZ' },
    date: '2099-09-09',
    time: '21:30',
    cost: '$20',
    ages: 'All Ages',
    description: 'AI flyer description',
  }

  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('seeds the extracted artist, venue, date, time, cost, ages, and description into the form at mount', () => {
    renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    // defaultValues are read synchronously at mount — no rAF / waitFor needed.
    expect(
      (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
    ).toBe('AI Artist')
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe('AI Venue')
    expect((screen.getByLabelText(/^City$/i) as HTMLInputElement).value).toBe('Tempe')
    expect((screen.getByLabelText(/^State$/i) as HTMLInputElement).value).toBe('AZ')
    expect((screen.getByLabelText(/^Date$/i) as HTMLInputElement).value).toBe('2099-09-09')
    expect((screen.getByLabelText(/^Time$/i) as HTMLInputElement).value).toBe('21:30')
    expect((screen.getByLabelText(/cost/i) as HTMLInputElement).value).toBe('$20')
    expect((screen.getByLabelText(/ages/i) as HTMLInputElement).value).toBe('All Ages')
    expect((screen.getByLabelText(/description/i) as HTMLTextAreaElement).value).toBe(
      'AI flyer description'
    )
  })

  it('re-seeds the form when a new extraction arrives with a changed key (remount)', async () => {
    const { rerender } = renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    expect(
      (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
    ).toBe('AI Artist')

    // A second extraction. The parent bumps `key`, so React unmounts the old
    // form and mounts a fresh one whose defaultValues come from the NEW data.
    const secondExtraction: ExtractedShowData = {
      artists: [{ name: 'Second Artist', is_headliner: true }],
      venue: { name: 'Second Venue', city: 'Mesa', state: 'AZ' },
      date: '2099-10-10',
      time: '19:00',
      cost: 'Free',
      ages: '21+',
      description: 'second flyer',
    }

    rerender(
      <ShowForm key={1} mode="create" initialExtraction={secondExtraction} />
    )

    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
      ).toBe('Second Artist')
    )
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe('Second Venue')
    expect((screen.getByLabelText(/^City$/i) as HTMLInputElement).value).toBe('Mesa')
    expect((screen.getByLabelText(/^Date$/i) as HTMLInputElement).value).toBe('2099-10-10')
    expect((screen.getByLabelText(/cost/i) as HTMLInputElement).value).toBe('Free')
  })

  it('does not clobber a user edit when re-rendered with the same key (defaultValues read once)', async () => {
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    const venueInput = screen.getByLabelText(/^Venue$/i) as HTMLInputElement
    expect(venueInput.value).toBe('AI Venue')

    // User edits the seeded value.
    await user.clear(venueInput)
    await user.type(venueInput, 'User Edited Venue')
    expect(venueInput.value).toBe('User Edited Venue')

    // Re-render with the SAME key and the SAME extraction (e.g. an unrelated
    // parent re-render). Without a key change the form is not remounted, so
    // defaultValues are not re-read and the user's edit must survive.
    rerender(<ShowForm key={0} mode="create" initialExtraction={extraction} />)

    await Promise.resolve()
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe(
      'User Edited Venue'
    )
  })
})

describe('ShowForm — edit mode pre-fills from initialData', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('populates artist, venue (locked label), city, state, ages, description, and price-as-cost from the show prop', () => {
    const show = makeShow()
    renderWithProviders(<ShowForm mode="edit" initialData={show} />)

    // Artist row carries the existing name.
    expect(
      screen.getByPlaceholderText('Enter artist name') as HTMLInputElement
    ).toHaveValue('The Mountain Goats')
    // Venue: edit mode goes through the VenueInput combobox (no prefilledVenue).
    expect(screen.getByLabelText(/^Venue$/i)).toHaveValue('Valley Bar')
    expect(screen.getByLabelText(/^City$/i)).toHaveValue('Phoenix')
    expect(screen.getByLabelText(/^State$/i)).toHaveValue('AZ')
    expect(screen.getByLabelText(/ages/i)).toHaveValue('21+')
    expect(screen.getByLabelText(/description/i)).toHaveValue('A pre-existing show.')
    // price=25 → cost field renders as "$25".
    expect(screen.getByLabelText(/cost/i)).toHaveValue('$25')
    // image_url is editable in edit mode (PSY-521 carve-out).
    expect(screen.getByLabelText(/image url/i)).toHaveValue('https://example.com/flyer.jpg')
    // Submit copy switches to "Save Changes".
    expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument()
  })

  it('renders Cancel button when onCancel is provided in edit mode', async () => {
    const onCancel = vi.fn()
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow()} onCancel={onCancel} />
    )

    const cancelButton = screen.getByRole('button', { name: /cancel/i })
    expect(cancelButton).toBeInTheDocument()
    await user.click(cancelButton)
    expect(onCancel).toHaveBeenCalledTimes(1)
  })
})

describe('ShowForm — private-show toggle visibility (create vs edit)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('IS visible in create mode when a non-admin enters a new (unmatched) venue', async () => {
    // Positive branch: venue is non-empty AND no matched venue AND user is
    // not admin AND !isEditMode. That combination satisfies every gate the
    // private-show checkbox sits behind in ShowForm.tsx.
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    // Typing into the venue input fires onVenueNameChange → setVenueName,
    // which is the load-bearing trigger for the "New Venue" branch.
    await user.type(screen.getByLabelText(/^Venue$/i), 'Brand New Spot')
    // Trigger blur — the component reads field.state.value to decide whether
    // selectedVenue should be cleared (it stays null because no match).
    await user.tab()

    expect(
      await screen.findByLabelText(/do not publish/i)
    ).toBeInTheDocument()
  })

  it('is NOT visible in edit mode even with a non-admin + new-venue-like state', async () => {
    // Negative branch: the same conditions that surface the checkbox in
    // create mode must NOT surface it in edit mode. ShowForm gates the
    // checkbox specifically on `!isEditMode`; this test pins that.
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    // Clear the prefilled venue text to drop the selected venue to null —
    // this puts the form into the same "new venue" state the create-mode
    // positive test relies on. If the only thing gating the toggle were
    // venue state, it would now be visible. Edit mode must still hide it.
    const venueInput = screen.getByLabelText(/^Venue$/i)
    await user.clear(venueInput)
    await user.type(venueInput, 'Different Venue')
    await user.tab()

    // Pin the precondition: the "New Venue" banner IS showing (proves we
    // reached the surrounding conditional). Without this, the absence
    // assertion below would also pass if the banner never rendered at all
    // (false pass from a setup failure rather than from the !isEditMode
    // gate firing).
    expect(await screen.findByText(/New Venue/i)).toBeInTheDocument()

    // The "New Venue" admin-info banner uses similar wording; assert
    // specifically on the toggle's own label so we're testing the
    // !isEditMode gate, not the outer banner.
    expect(screen.queryByLabelText(/do not publish/i)).not.toBeInTheDocument()
  })
})
