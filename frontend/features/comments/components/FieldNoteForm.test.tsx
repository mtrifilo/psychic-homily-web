import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FieldNoteForm } from './FieldNoteForm'

describe('FieldNoteForm', () => {
  it('renders the form with textarea and submit button', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} />)

    expect(screen.getByTestId('field-note-form')).toBeInTheDocument()
    expect(screen.getByTestId('field-note-textarea')).toBeInTheDocument()
    expect(screen.getByTestId('field-note-submit')).toBeInTheDocument()
  })

  it('renders disabled message when disabled with message', () => {
    render(
      <FieldNoteForm
        onSubmit={vi.fn()}
        disabled
        disabledMessage="Field notes are available after the show"
      />
    )

    expect(screen.getByTestId('field-note-form-disabled')).toBeInTheDocument()
    expect(screen.getByText('Field notes are available after the show')).toBeInTheDocument()
    expect(screen.queryByTestId('field-note-form')).not.toBeInTheDocument()
  })

  it('submit button is disabled when textarea is empty', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} />)

    expect(screen.getByTestId('field-note-submit')).toBeDisabled()
  })

  it('submit button is enabled when textarea has content', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Great show!' },
    })

    expect(screen.getByTestId('field-note-submit')).not.toBeDisabled()
  })

  it('calls onSubmit with trimmed body but does NOT clear form (PSY-608 — clear is parent-driven via resetSignal)', () => {
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: '  Amazing performance  ' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ body: 'Amazing performance' })
    )
    // PSY-608: form keeps the draft so 4xx errors don't discard typed text.
    // Parent clears via resetSignal on mutation success.
    expect(screen.getByTestId('field-note-textarea')).toHaveValue(
      '  Amazing performance  '
    )
  })

  it('clears form when parent bumps resetSignal (PSY-608)', () => {
    const handleSubmit = vi.fn()
    const { rerender } = render(
      <FieldNoteForm onSubmit={handleSubmit} resetSignal={0} />
    )

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'My note' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ body: 'My note' })
    )
    // Pre-bump: draft preserved.
    expect(screen.getByTestId('field-note-textarea')).toHaveValue('My note')

    // Parent signals success.
    rerender(<FieldNoteForm onSubmit={handleSubmit} resetSignal={1} />)
    expect(screen.getByTestId('field-note-textarea')).toHaveValue('')
  })

  it('renders an inline error banner when errorMessage is set (PSY-608)', () => {
    render(
      <FieldNoteForm
        onSubmit={vi.fn()}
        errorMessage="Please wait 60s before commenting again."
      />
    )
    const banner = screen.getByTestId('field-note-form-error')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveAttribute('role', 'alert')
    expect(banner).toHaveTextContent(
      'Please wait 60s before commenting again.'
    )
  })

  it('preserves draft when an errorMessage is present and no resetSignal bump (PSY-608)', () => {
    const handleSubmit = vi.fn()
    const { rerender } = render(
      <FieldNoteForm onSubmit={handleSubmit} resetSignal={0} />
    )

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'first try' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    // Mutation comes back 4xx — parent renders errorMessage but does NOT
    // bump resetSignal. The draft must survive.
    rerender(
      <FieldNoteForm
        onSubmit={handleSubmit}
        resetSignal={0}
        errorMessage="Please wait 60s before commenting again."
      />
    )

    expect(screen.getByTestId('field-note-form-error')).toBeInTheDocument()
    expect(screen.getByTestId('field-note-textarea')).toHaveValue('first try')
  })

  it('includes sound quality when set', async () => {
    const user = userEvent.setup()
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Good sound' },
    })

    // Click 4th star for sound quality
    const soundRating = screen.getByTestId('sound-quality-rating')
    const stars = soundRating.querySelectorAll('button')
    await user.click(stars[3]) // 4th star

    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        body: 'Good sound',
        sound_quality: 4,
      })
    )
  })

  it('includes crowd energy when set', async () => {
    const user = userEvent.setup()
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Energetic crowd' },
    })

    const crowdRating = screen.getByTestId('crowd-energy-rating')
    const stars = crowdRating.querySelectorAll('button')
    await user.click(stars[4]) // 5th star

    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        body: 'Energetic crowd',
        crowd_energy: 5,
      })
    )
  })

  it('includes notable moments when filled', () => {
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Great show' },
    })
    fireEvent.change(screen.getByTestId('notable-moments-input'), {
      target: { value: 'Played 3 new songs' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        body: 'Great show',
        notable_moments: 'Played 3 new songs',
      })
    )
  })

  it('renders artist picker when artists provided', () => {
    render(
      <FieldNoteForm
        onSubmit={vi.fn()}
        artists={[
          { id: 1, name: 'Band A' },
          { id: 2, name: 'Band B' },
        ]}
      />
    )

    const select = screen.getByTestId('artist-select')
    expect(select).toBeInTheDocument()
    expect(screen.getByText('Band A')).toBeInTheDocument()
    expect(screen.getByText('Band B')).toBeInTheDocument()
  })

  it('does not render artist picker when no artists', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} />)

    expect(screen.queryByTestId('artist-select')).not.toBeInTheDocument()
  })

  it('includes setlist_spoiler when checked', async () => {
    const user = userEvent.setup()
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'They opened with...' },
    })

    await user.click(screen.getByTestId('setlist-spoiler-checkbox'))
    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        body: 'They opened with...',
        setlist_spoiler: true,
      })
    )
  })

  it('disables form elements when isPending', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} isPending />)

    expect(screen.getByTestId('field-note-textarea')).toBeDisabled()
    expect(screen.getByTestId('field-note-submit')).toBeDisabled()
  })

  it('renders song position input', () => {
    render(<FieldNoteForm onSubmit={vi.fn()} />)

    expect(screen.getByTestId('song-position-input')).toBeInTheDocument()
  })

  it('includes song_position when set', () => {
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Amazing solo' },
    })
    fireEvent.change(screen.getByTestId('song-position-input'), {
      target: { value: '7' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    expect(handleSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        body: 'Amazing solo',
        song_position: 7,
      })
    )
  })

  it('does not include optional fields when empty (PSY-568: verified_attendee is always sent as the explicit user choice)', () => {
    const handleSubmit = vi.fn()
    render(<FieldNoteForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('field-note-textarea'), {
      target: { value: 'Simple note' },
    })
    fireEvent.click(screen.getByTestId('field-note-submit'))

    const call = handleSubmit.mock.calls[0][0]
    // PSY-568: verified_attendee is always included so the backend stores
    // the explicit user choice. Default is false (unchecked) when there is
    // no Going RSVP, matching the server-side default.
    expect(call).toEqual({ body: 'Simple note', verified_attendee: false })
    expect(call).not.toHaveProperty('sound_quality')
    expect(call).not.toHaveProperty('crowd_energy')
    expect(call).not.toHaveProperty('notable_moments')
    expect(call).not.toHaveProperty('setlist_spoiler')
  })

  // PSY-568: Self-claim verified-attendee checkbox.
  describe('PSY-568 verified-attendee checkbox', () => {
    it('renders the "I attended this show" checkbox', () => {
      render(<FieldNoteForm onSubmit={vi.fn()} />)

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toBeInTheDocument()
      expect(screen.getByText('I attended this show')).toBeInTheDocument()
    })

    it('checkbox is unchecked by default when defaultVerifiedAttendee is false (no Going RSVP)', () => {
      render(<FieldNoteForm onSubmit={vi.fn()} />)

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toHaveAttribute('aria-checked', 'false')
    })

    it('checkbox is pre-checked when defaultVerifiedAttendee is true (user has Going set)', () => {
      render(<FieldNoteForm onSubmit={vi.fn()} defaultVerifiedAttendee />)

      const checkbox = screen.getByTestId('verified-attendee-checkbox')
      expect(checkbox).toHaveAttribute('aria-checked', 'true')
    })

    it('sends verified_attendee=true when the user checks the box', async () => {
      const user = userEvent.setup()
      const handleSubmit = vi.fn()
      render(<FieldNoteForm onSubmit={handleSubmit} />)

      fireEvent.change(screen.getByTestId('field-note-textarea'), {
        target: { value: 'I was there.' },
      })
      await user.click(screen.getByTestId('verified-attendee-checkbox'))
      fireEvent.click(screen.getByTestId('field-note-submit'))

      expect(handleSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          body: 'I was there.',
          verified_attendee: true,
        })
      )
    })

    it('sends verified_attendee=false when the user unchecks a pre-checked box (override)', async () => {
      const user = userEvent.setup()
      const handleSubmit = vi.fn()
      render(<FieldNoteForm onSubmit={handleSubmit} defaultVerifiedAttendee />)

      fireEvent.change(screen.getByTestId('field-note-textarea'), {
        target: { value: 'Marked Going but couldn\'t make it.' },
      })
      // Box pre-checked; user unticks it.
      await user.click(screen.getByTestId('verified-attendee-checkbox'))
      fireEvent.click(screen.getByTestId('field-note-submit'))

      expect(handleSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          verified_attendee: false,
        })
      )
    })

    it('re-syncs the checkbox when defaultVerifiedAttendee changes (attendance loads after mount)', () => {
      const { rerender } = render(<FieldNoteForm onSubmit={vi.fn()} />)
      expect(screen.getByTestId('verified-attendee-checkbox')).toHaveAttribute(
        'aria-checked',
        'false'
      )

      // Attendance query resolves and the parent passes defaultVerifiedAttendee=true.
      rerender(<FieldNoteForm onSubmit={vi.fn()} defaultVerifiedAttendee />)
      expect(screen.getByTestId('verified-attendee-checkbox')).toHaveAttribute(
        'aria-checked',
        'true'
      )
    })
  })
})
