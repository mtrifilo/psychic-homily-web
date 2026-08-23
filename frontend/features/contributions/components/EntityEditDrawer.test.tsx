import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-599: client-side URL pre-validation in the suggest-edit drawer.
// Tests the disabled-Submit + inline-error behavior. The validator helper
// itself is covered exhaustively in `types.test.ts` — these tests just
// exercise the wiring into the drawer's existing canSubmit logic.

const mockMutate = vi.fn()
const mockReset = vi.fn()

vi.mock('../hooks/useSuggestEdit', () => ({
  useSuggestEdit: () => ({
    mutate: mockMutate,
    reset: mockReset,
    isPending: false,
    isSuccess: false,
    isError: false,
    data: undefined as unknown,
    error: null as Error | null,
  }),
}))

import { EntityEditDrawer } from './EntityEditDrawer'

describe('EntityEditDrawer URL validation (PSY-599)', () => {
  const baseEntity = {
    name: 'Amyl and the Sniffers',
    instagram: '',
    facebook: '',
  }

  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    entityType: 'artist' as const,
    entityId: 42,
    entityName: 'Amyl and the Sniffers',
    entity: baseEntity,
    canEditDirectly: false,
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  function fillSummary() {
    const summaryField = screen.getByLabelText(/Why are you making this change/)
    fireEvent.change(summaryField, { target: { value: 'Add Instagram link' } })
  }

  function getSubmitButton() {
    return screen.getByRole('button', { name: /Submit for Review/i })
  }

  // The summary is served on anonymous revision-history routes with no edit or
  // delete endpoint behind it. The old copy ("Helps reviewers understand your
  // edit") told the contributor the audience was moderators, so the natural
  // summary for an address correction contained the address. Asserting on the
  // words the contributor has to see, not on a test id, is the point: reverting
  // to reviewer-only framing must fail here.
  it('warns that the edit summary is published publicly and permanently', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    const helper = screen.getByText(/published in the public edit history/i)
    expect(helper).toBeInTheDocument()
    expect(helper).toHaveTextContent(/cannot be changed or removed later/i)
    expect(helper).toHaveTextContent(/leave out private details/i)
    expect(screen.queryByText(/Helps reviewers understand your edit/i)).not.toBeInTheDocument()
  })

  it('disables Submit when an invalid URL is typed into a url field', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    // Fill summary so it's not the blocker.
    fillSummary()

    // Type a malformed URL — this is the canonical PSY-599 input.
    const instagramInput = screen.getByTestId('edit-instagram-input')
    fireEvent.change(instagramInput, { target: { value: 'not-a-real-url' } })

    expect(getSubmitButton()).toBeDisabled()
  })

  it('shows an inline error after the user touches a malformed URL field', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    const instagramInput = screen.getByTestId('edit-instagram-input')
    fireEvent.change(instagramInput, { target: { value: 'not-a-real-url' } })

    const error = screen.getByTestId('edit-instagram-error')
    expect(error).toBeInTheDocument()
    expect(error.textContent).toMatch(/http/i)
    // aria-invalid wires the error to the input for screen readers.
    expect(instagramInput).toHaveAttribute('aria-invalid', 'true')
  })

  it('enables Submit once the URL becomes valid', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    fillSummary()

    const instagramInput = screen.getByTestId('edit-instagram-input')
    // Start invalid.
    fireEvent.change(instagramInput, { target: { value: 'not-a-real-url' } })
    expect(getSubmitButton()).toBeDisabled()

    // Replace with a valid URL.
    fireEvent.change(instagramInput, {
      target: { value: 'https://instagram.com/amylandthesniffers' },
    })

    expect(getSubmitButton()).toBeEnabled()
    expect(screen.queryByTestId('edit-instagram-error')).not.toBeInTheDocument()
  })

  it('disables Submit when one of multiple URL fields is invalid', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    fillSummary()

    // Valid Instagram, malformed Facebook → still blocked.
    const instagramInput = screen.getByTestId('edit-instagram-input')
    fireEvent.change(instagramInput, {
      target: { value: 'https://instagram.com/x' },
    })
    const facebookInput = screen.getByTestId('edit-facebook-input')
    fireEvent.change(facebookInput, { target: { value: 'fb.com/x' } })

    expect(getSubmitButton()).toBeDisabled()
  })

  it('rejects javascript: scheme client-side', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    fillSummary()

    const instagramInput = screen.getByTestId('edit-instagram-input')
    fireEvent.change(instagramInput, { target: { value: 'javascript:alert(1)' } })

    expect(getSubmitButton()).toBeDisabled()
    expect(screen.getByTestId('edit-instagram-error')).toBeInTheDocument()
  })

  it('does not block Submit on text fields with no URL constraint', () => {
    renderWithProviders(<EntityEditDrawer {...defaultProps} />)

    fillSummary()

    // Change a non-URL field — name is plain text. "Anything goes" is fine.
    const nameInput = screen.getByLabelText(/^Name$/) as HTMLInputElement
    fireEvent.change(nameInput, { target: { value: 'Amyl & The Sniffers' } })

    expect(getSubmitButton()).toBeEnabled()
  })

  it('does not flag a pre-existing invalid URL the user has not modified', () => {
    // Edge: the entity may already have a non-conforming URL persisted from
    // before PSY-525 / PSY-549. We must not block edits to OTHER fields just
    // because the existing record fails the new rule.
    renderWithProviders(
      <EntityEditDrawer
        {...defaultProps}
        entity={{ ...baseEntity, instagram: 'not-a-real-url' }}
      />
    )

    fillSummary()

    // Touch an unrelated field so changes is non-empty.
    const nameInput = screen.getByLabelText(/^Name$/) as HTMLInputElement
    fireEvent.change(nameInput, { target: { value: 'Amyl & The Sniffers' } })

    expect(getSubmitButton()).toBeEnabled()
    // No inline error should appear unless the user actually edits the URL.
    expect(screen.queryByTestId('edit-instagram-error')).not.toBeInTheDocument()
  })
})

// PSY-1682: the venue's HOUSE DEFAULT age policy is curated through the same
// suggest-edit drawer as every other venue text field. The per-event override
// lives on the show's `age_requirement` and is edited on the show, not here.
describe('EntityEditDrawer venue age policy (PSY-1682)', () => {
  const venueProps = {
    open: true,
    onOpenChange: vi.fn(),
    entityType: 'venue' as const,
    entityId: 7,
    entityName: 'Crescent Ballroom',
    entity: { name: 'Crescent Ballroom', age_policy: '' } as Record<string, unknown>,
    canEditDirectly: false,
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  function fillSummary(text = 'Confirmed the house age policy with the venue') {
    fireEvent.change(screen.getByLabelText(/Why are you making this change/), {
      target: { value: text },
    })
  }

  it('exposes an Age Policy input on the venue drawer', () => {
    renderWithProviders(<EntityEditDrawer {...venueProps} />)

    expect(screen.getByTestId('edit-age_policy-input')).toBeInTheDocument()
    expect(screen.getByLabelText(/^Age Policy$/)).toBeInTheDocument()
  })

  it('submits a newly set age policy as a field change', () => {
    renderWithProviders(<EntityEditDrawer {...venueProps} />)

    fireEvent.change(screen.getByTestId('edit-age_policy-input'), {
      target: { value: 'all ages' },
    })
    fillSummary()

    const submit = screen.getByRole('button', { name: /Submit for Review/i })
    expect(submit).toBeEnabled()
    fireEvent.click(submit)

    expect(mockMutate).toHaveBeenCalledTimes(1)
    const payload = mockMutate.mock.calls[0][0]
    expect(payload.entityType).toBe('venue')
    expect(payload.entityId).toBe(7)
    expect(payload.changes).toEqual([
      { field: 'age_policy', old_value: null, new_value: 'all ages' },
    ])
  })

  it('sends new_value null when an existing age policy is cleared', () => {
    // Clearing is the gesture that says "this room has no standing rule".
    // It must reach the server as null so the column goes back to NULL rather
    // than persisting an empty string that reads as a known blank policy.
    renderWithProviders(
      <EntityEditDrawer
        {...venueProps}
        entity={{ name: 'Crescent Ballroom', age_policy: '21+' }}
      />
    )

    const input = screen.getByTestId('edit-age_policy-input') as HTMLInputElement
    expect(input.value).toBe('21+')

    fireEvent.change(input, { target: { value: '' } })
    fillSummary('Venue dropped its 21+ policy')

    fireEvent.click(screen.getByRole('button', { name: /Submit for Review/i }))

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(mockMutate.mock.calls[0][0].changes).toEqual([
      { field: 'age_policy', old_value: '21+', new_value: null },
    ])
  })
})

// PSY-1694: room capacity is the drawer's first NUMERIC field, so it is the
// first one whose change is submitted as a JSON number rather than as the
// string the input holds. The column is an integer and NULL is how "we do not
// know" is stored, which makes both the coercion and the clear gesture
// load-bearing rather than cosmetic.
describe('EntityEditDrawer venue capacity (PSY-1694)', () => {
  const venueProps = {
    open: true,
    onOpenChange: vi.fn(),
    entityType: 'venue' as const,
    entityId: 7,
    entityName: 'Crescent Ballroom',
    entity: { name: 'Crescent Ballroom', capacity: null } as Record<string, unknown>,
    canEditDirectly: false,
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  function fillSummary(text = 'Confirmed the room capacity with the venue') {
    fireEvent.change(screen.getByLabelText(/Why are you making this change/), {
      target: { value: text },
    })
  }

  function getSubmitButton() {
    return screen.getByRole('button', { name: /Submit for Review/i })
  }

  it('exposes a Capacity input on the venue drawer', () => {
    renderWithProviders(<EntityEditDrawer {...venueProps} />)

    const input = screen.getByTestId('edit-capacity-input')
    expect(input).toBeInTheDocument()
    expect(screen.getByLabelText(/^Capacity$/)).toBeInTheDocument()
    // Kept as type="text" with a numeric inputMode: type="number" reports ""
    // for anything the browser considers invalid, which would make a typo
    // indistinguishable from the clear gesture on a field where clearing
    // means NULL.
    expect(input).toHaveAttribute('type', 'text')
    expect(input).toHaveAttribute('inputmode', 'numeric')
  })

  it('pre-fills the existing capacity as a string and submits it as a number', () => {
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 550 }} />
    )

    const input = screen.getByTestId('edit-capacity-input') as HTMLInputElement
    expect(input.value).toBe('550')

    fireEvent.change(input, { target: { value: '3600' } })
    fillSummary()
    fireEvent.click(getSubmitButton())

    expect(mockMutate).toHaveBeenCalledTimes(1)
    const payload = mockMutate.mock.calls[0][0]
    expect(payload.entityType).toBe('venue')
    // Numbers on the wire, not strings: the backend rejects a numeric string
    // for this field rather than parsing it.
    expect(payload.changes).toEqual([{ field: 'capacity', old_value: 550, new_value: 3600 }])
  })

  it('sends new_value null when an existing capacity is cleared', () => {
    // The clear gesture must reach the column as NULL. A 0 would read
    // downstream as a known capacity of zero.
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 550 }} />
    )

    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: '' } })
    fillSummary('The published number turned out to be wrong')
    fireEvent.click(getSubmitButton())

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(mockMutate.mock.calls[0][0].changes).toEqual([
      { field: 'capacity', old_value: 550, new_value: null },
    ])
  })

  it('blocks Submit and shows an inline error for zero, negatives and fractions', () => {
    renderWithProviders(<EntityEditDrawer {...venueProps} />)
    fillSummary()

    const input = screen.getByTestId('edit-capacity-input')
    for (const bad of ['0', '-40', '550.5', '200001', 'lots']) {
      fireEvent.change(input, { target: { value: bad } })
      expect(getSubmitButton()).toBeDisabled()
      expect(screen.getByTestId('edit-capacity-error')).toBeInTheDocument()
      expect(input).toHaveAttribute('aria-invalid', 'true')
    }

    // A legal value clears the block.
    fireEvent.change(input, { target: { value: '550' } })
    expect(getSubmitButton()).toBeEnabled()
    expect(screen.queryByTestId('edit-capacity-error')).not.toBeInTheDocument()
  })

  it('does not block edits to other fields when capacity is untouched', () => {
    // A pre-existing out-of-range capacity (from ingest, say) must not hold the
    // whole drawer hostage, matching how pre-existing bad URLs are tolerated.
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 0 }} />
    )

    // The tolerated bad value is really in the form, not merely absent.
    expect((screen.getByTestId('edit-capacity-input') as HTMLInputElement).value).toBe('0')

    fillSummary('Fix the venue name')
    fireEvent.change(screen.getByLabelText(/^Name$/), {
      target: { value: 'Crescent Ballroom PHX' },
    })

    expect(getSubmitButton()).toBeEnabled()
    expect(screen.queryByTestId('edit-capacity-error')).not.toBeInTheDocument()
    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('renders a falsy numeric value in the preview instead of calling it cleared', () => {
    // The preview used to test old/new for TRUTHINESS, so 0 rendered as
    // "cleared" -- a different edit from the one the contributor made. Only
    // null means cleared.
    renderWithProviders(<EntityEditDrawer {...venueProps} />)

    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: '0' } })

    expect(screen.getByText('Changes Preview')).toBeInTheDocument()
    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.queryByText('cleared')).not.toBeInTheDocument()
  })

  it('files no change when a numeric edit is only cosmetic', () => {
    // Change detection compares the values that get SENT, not the raw input.
    // "550 " and "550" are the same capacity, and submitting the difference
    // would put a "550 -> 550" row in front of an admin.
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 550 }} />
    )

    fillSummary()
    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: ' 550 ' } })

    expect(screen.queryByText('Changes Preview')).not.toBeInTheDocument()
    expect(getSubmitButton()).toBeDisabled()
  })

  it('labels a cosmetic numeric edit as unchanged, so Submit is never silently dead', () => {
    // The "(changed)" affordance and the payload must agree. Deriving the label
    // from the raw string instead would paint the field blue, show no error,
    // and still refuse to submit with nothing on screen explaining why.
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 550 }} />
    )

    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: '550 ' } })

    expect(screen.queryByText('(changed)')).not.toBeInTheDocument()
    expect(screen.queryByTestId('edit-capacity-error')).not.toBeInTheDocument()
  })

  it('names the clear gesture in the preview instead of showing a bare strikethrough', () => {
    renderWithProviders(
      <EntityEditDrawer {...venueProps} entity={{ name: 'Crescent Ballroom', capacity: 550 }} />
    )

    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: '' } })

    expect(screen.getByText('550')).toBeInTheDocument()
    expect(screen.getByText('cleared')).toBeInTheDocument()
  })

  it('files no change when whitespace is typed into an already empty capacity', () => {
    // Both sides convert to null, so this clears nothing and must not reach
    // the review queue as a spurious "cleared".
    renderWithProviders(<EntityEditDrawer {...venueProps} />)

    fillSummary()
    fireEvent.change(screen.getByTestId('edit-capacity-input'), { target: { value: '   ' } })

    expect(screen.queryByText('Changes Preview')).not.toBeInTheDocument()
    expect(getSubmitButton()).toBeDisabled()
  })
})

// PSY-1664: an applied (direct) edit flashes success in the drawer, then closes
// itself on a delay. That delay used to be a bare `setTimeout`, so it still
// fired after the drawer unmounted and called `onOpenChange` / `onSuccess` into
// a torn-down React DOM. The invariant is that no timer survives unmount.
describe('EntityEditDrawer applied-close timer cleanup (PSY-1664)', () => {
  const directEditProps = {
    open: true,
    onOpenChange: vi.fn(),
    entityType: 'artist' as const,
    entityId: 42,
    entityName: 'Amyl and the Sniffers',
    entity: { name: 'Amyl and the Sniffers', instagram: '' } as Record<string, unknown>,
    canEditDirectly: true,
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('leaves no pending close timer behind on unmount', () => {
    vi.useFakeTimers()
    try {
      // The drawer only arms the close timer when the mutation reports the
      // edit was applied directly rather than queued for review.
      mockMutate.mockImplementation(
        (
          _vars: unknown,
          opts?: { onSuccess?: (data: { applied: boolean }) => void }
        ) => {
          opts?.onSuccess?.({ applied: true })
        }
      )

      const { unmount } = renderWithProviders(
        <EntityEditDrawer {...directEditProps} onSuccess={vi.fn()} />
      )

      // The Sheet the drawer renders in keeps its own animation timers alive
      // across unmount, so this measures the drawer's delta rather than an
      // absolute zero.
      const baseline = vi.getTimerCount()

      fireEvent.change(screen.getByLabelText(/^Name$/), {
        target: { value: 'Amyl & The Sniffers' },
      })
      fireEvent.change(screen.getByLabelText(/Why are you making this change/), {
        target: { value: 'Fix the ampersand in the name' },
      })
      fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(mockMutate).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBeGreaterThan(baseline)

      unmount()
      expect(vi.getTimerCount()).toBeLessThanOrEqual(baseline)
    } finally {
      vi.useRealTimers()
    }
  })
})
