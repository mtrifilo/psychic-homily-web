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
    data: undefined,
    error: null,
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
