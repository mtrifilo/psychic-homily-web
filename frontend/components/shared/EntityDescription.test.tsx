import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { EntityDescription } from './EntityDescription'

describe('EntityDescription', () => {
  const defaultOnSave = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    defaultOnSave.mockResolvedValue(undefined)
  })

  // ── Display mode ──────────────────────────────────────────────────

  describe('display mode', () => {
    it('renders description text', () => {
      renderWithProviders(
        <EntityDescription
          description="A great venue in downtown Phoenix."
          canEdit={false}
          onSave={defaultOnSave}
        />
      )

      expect(
        screen.getByText('A great venue in downtown Phoenix.')
      ).toBeInTheDocument()
    })

    it('renders paragraphs from double newlines', () => {
      renderWithProviders(
        <EntityDescription
          description={'First paragraph.\n\nSecond paragraph.'}
          canEdit={false}
          onSave={defaultOnSave}
        />
      )

      expect(screen.getByText('First paragraph.')).toBeInTheDocument()
      expect(screen.getByText('Second paragraph.')).toBeInTheDocument()

      // They should be in separate <p> elements
      const paragraphs = screen
        .getByText('First paragraph.')
        .closest('p')!
      const secondParagraph = screen
        .getByText('Second paragraph.')
        .closest('p')!
      expect(paragraphs).not.toBe(secondParagraph)
    })

    it('preserves single newlines as line breaks', () => {
      const { container } = renderWithProviders(
        <EntityDescription
          description={'Line one\nLine two'}
          canEdit={false}
          onSave={defaultOnSave}
        />
      )

      // Single newline should produce a <br> within the same <p>
      const paragraph = screen.getByText('Line one').closest('p')!
      expect(paragraph).toContainHTML('<br')
      expect(paragraph).toHaveTextContent('Line oneLine two')
    })
  })

  // ── Empty state ───────────────────────────────────────────────────

  describe('empty state', () => {
    it('shows "No description yet" with "Add description" when canEdit=true', () => {
      renderWithProviders(
        <EntityDescription
          description={null}
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      expect(screen.getByText(/No description yet/)).toBeInTheDocument()
      expect(screen.getByText('Add description')).toBeInTheDocument()
    })

    it('shows empty state for empty string description when canEdit=true', () => {
      renderWithProviders(
        <EntityDescription
          description="   "
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      expect(screen.getByText(/No description yet/)).toBeInTheDocument()
    })

    it('shows empty state for undefined description when canEdit=true', () => {
      renderWithProviders(
        <EntityDescription
          description={undefined}
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      expect(screen.getByText(/No description yet/)).toBeInTheDocument()
    })

    it('returns null when no description and canEdit=false', () => {
      const { container } = renderWithProviders(
        <EntityDescription
          description={null}
          canEdit={false}
          onSave={defaultOnSave}
        />
      )

      expect(container.innerHTML).toBe('')
    })
  })

  // ── Edit button ───────────────────────────────────────────────────

  describe('edit button', () => {
    it('shows edit button when canEdit=true and description exists', () => {
      renderWithProviders(
        <EntityDescription
          description="Existing description"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      expect(
        screen.getByRole('button', { name: /edit description/i })
      ).toBeInTheDocument()
    })

    it('does not show edit button when canEdit=false', () => {
      renderWithProviders(
        <EntityDescription
          description="Existing description"
          canEdit={false}
          onSave={defaultOnSave}
        />
      )

      expect(
        screen.queryByRole('button', { name: /edit description/i })
      ).not.toBeInTheDocument()
    })
  })

  // ── Edit mode ─────────────────────────────────────────────────────

  describe('edit mode', () => {
    it('clicking edit shows textarea with current description', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Current text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      const textarea = screen.getByRole('textbox')
      expect(textarea).toBeInTheDocument()
      expect(textarea).toHaveValue('Current text')
    })

    it('clicking edit shows Save and Cancel buttons', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Current text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      expect(
        screen.getByRole('button', { name: /save/i })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /cancel/i })
      ).toBeInTheDocument()
    })

    it('clicking "Add description" empty state enters edit mode', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description={null}
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(screen.getByText(/No description yet/))

      expect(screen.getByRole('textbox')).toBeInTheDocument()
      expect(screen.getByRole('textbox')).toHaveValue('')
    })
  })

  // ── Character counter ─────────────────────────────────────────────

  describe('character counter', () => {
    it('shows character count in edit mode', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Hello"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      expect(screen.getByText(/5 \/ 5,000/)).toBeInTheDocument()
    })

    it('updates character count as user types', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description=""
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      // Empty string is treated as no description, so we get the "Add description" empty state
      await user.click(screen.getByText(/No description yet/))

      expect(screen.getByText(/0 \/ 5,000/)).toBeInTheDocument()

      await user.type(screen.getByRole('textbox'), 'abc')

      expect(screen.getByText(/3 \/ 5,000/)).toBeInTheDocument()
    })
  })

  // ── Cancel ────────────────────────────────────────────────────────

  describe('cancel', () => {
    it('clicking Cancel returns to display mode without saving', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Original text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      // Modify the textarea
      await user.clear(screen.getByRole('textbox'))
      await user.type(screen.getByRole('textbox'), 'Changed text')

      // Click cancel
      await user.click(screen.getByRole('button', { name: /cancel/i }))

      // Should be back in display mode with original text
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText('Original text')).toBeInTheDocument()
      expect(defaultOnSave).not.toHaveBeenCalled()
    })
  })

  // ── Save ──────────────────────────────────────────────────────────

  describe('save', () => {
    it('clicking Save calls onSave with trimmed textarea content', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Old text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      await user.clear(screen.getByRole('textbox'))
      await user.type(screen.getByRole('textbox'), '  New text  ')

      await user.click(screen.getByRole('button', { name: /save/i }))

      expect(defaultOnSave).toHaveBeenCalledWith('New text')
    })

    it('returns to display mode after successful save', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Old text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      await user.clear(screen.getByRole('textbox'))
      await user.type(screen.getByRole('textbox'), 'New text')

      await user.click(screen.getByRole('button', { name: /save/i }))

      await waitFor(() => {
        expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      })
    })

    it('shows loading state while saving', async () => {
      // Make onSave hang so we can observe loading
      let resolveSave: () => void
      const savePending = new Promise<void>((resolve) => {
        resolveSave = resolve
      })
      const onSaveSlow = vi.fn().mockReturnValue(savePending)

      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Text"
          canEdit={true}
          onSave={onSaveSlow}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      await user.click(screen.getByRole('button', { name: /save/i }))

      // Save and Cancel buttons should be disabled while saving
      expect(screen.getByRole('button', { name: /save/i })).toBeDisabled()
      expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()

      // Resolve the save to clean up
      resolveSave!()
      await waitFor(() => {
        expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      })
    })
  })

  // ── Save error ────────────────────────────────────────────────────

  describe('save error', () => {
    it('shows error message when onSave rejects with Error', async () => {
      const onSaveFail = vi
        .fn()
        .mockRejectedValue(new Error('Network error'))

      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Text"
          canEdit={true}
          onSave={onSaveFail}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      await user.click(screen.getByRole('button', { name: /save/i }))

      await waitFor(() => {
        expect(screen.getByText('Network error')).toBeInTheDocument()
      })

      // Should remain in edit mode
      expect(screen.getByRole('textbox')).toBeInTheDocument()
    })

    it('shows fallback error message for non-Error rejection', async () => {
      const onSaveFail = vi.fn().mockRejectedValue('string error')

      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Text"
          canEdit={true}
          onSave={onSaveFail}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      await user.click(screen.getByRole('button', { name: /save/i }))

      await waitFor(() => {
        expect(
          screen.getByText('Failed to save description')
        ).toBeInTheDocument()
      })
    })

    it('shows validation error when description exceeds max length', async () => {
      const user = userEvent.setup()

      renderWithProviders(
        <EntityDescription
          description="Text"
          canEdit={true}
          onSave={defaultOnSave}
        />
      )

      await user.click(
        screen.getByRole('button', { name: /edit description/i })
      )

      // Directly set a value longer than 5000 chars via fireEvent
      // (userEvent.type for 5001 chars would be very slow)
      const textarea = screen.getByRole('textbox')
      // Bypass the HTML maxLength attribute by setting the value natively
      const longText = 'a'.repeat(5001)
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        window.HTMLTextAreaElement.prototype,
        'value'
      )!.set!
      nativeInputValueSetter.call(textarea, longText)
      textarea.dispatchEvent(new Event('input', { bubbles: true }))
      textarea.dispatchEvent(new Event('change', { bubbles: true }))

      await user.click(screen.getByRole('button', { name: /save/i }))

      // The error message uses the raw number (no toLocaleString)
      await waitFor(() => {
        expect(
          screen.getByText(/5000 characters or fewer/)
        ).toBeInTheDocument()
      })

      expect(defaultOnSave).not.toHaveBeenCalled()
    })
  })
})
