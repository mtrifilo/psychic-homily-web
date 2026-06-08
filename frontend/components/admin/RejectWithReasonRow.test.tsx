import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'

import { RejectWithReasonRow } from './RejectWithReasonRow'

function setup(overrides?: Partial<Parameters<typeof RejectWithReasonRow>[0]>) {
  const onApprove = vi.fn()
  const onReject = vi.fn()
  render(
    <RejectWithReasonRow
      onApprove={onApprove}
      onReject={onReject}
      isActioning={false}
      isApproving={false}
      isRejecting={false}
      rejectPlaceholder="Rejection reason (required)"
      {...overrides}
    />
  )
  return { onApprove, onReject }
}

describe('RejectWithReasonRow', () => {
  it('renders Approve + Reject in the resting state', () => {
    setup()

    expect(screen.getByRole('button', { name: /approve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^reject$/i })).toBeInTheDocument()
    // No textarea until the admin chooses to reject.
    expect(screen.queryByPlaceholderText('Rejection reason (required)')).not.toBeInTheDocument()
  })

  it('fires onApprove immediately (no confirmation step)', () => {
    const { onApprove } = setup()

    fireEvent.click(screen.getByRole('button', { name: /approve/i }))

    expect(onApprove).toHaveBeenCalledTimes(1)
  })

  it('expands a required-reason textarea when Reject is clicked', () => {
    setup()

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))

    expect(screen.getByPlaceholderText('Rejection reason (required)')).toBeInTheDocument()
    // Resting Approve/Reject are replaced by Confirm Reject + Cancel.
    expect(screen.getByRole('button', { name: /confirm reject/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /approve/i })).not.toBeInTheDocument()
  })

  it('keeps Confirm Reject disabled until a non-whitespace reason is entered', () => {
    setup()

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    const confirm = screen.getByRole('button', { name: /confirm reject/i })
    expect(confirm).toBeDisabled()

    // Whitespace-only does not enable it.
    fireEvent.change(screen.getByPlaceholderText('Rejection reason (required)'), {
      target: { value: '   ' },
    })
    expect(confirm).toBeDisabled()

    fireEvent.change(screen.getByPlaceholderText('Rejection reason (required)'), {
      target: { value: 'Inaccurate change' },
    })
    expect(confirm).not.toBeDisabled()
  })

  it('passes the TRIMMED reason to onReject on confirm', () => {
    const { onReject } = setup()

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    fireEvent.change(screen.getByPlaceholderText('Rejection reason (required)'), {
      target: { value: '  Spam content  ' },
    })
    fireEvent.click(screen.getByRole('button', { name: /confirm reject/i }))

    expect(onReject).toHaveBeenCalledWith('Spam content')
  })

  it('returns to the resting state when Cancel is clicked, discarding the reason', () => {
    const { onReject } = setup()

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    fireEvent.change(screen.getByPlaceholderText('Rejection reason (required)'), {
      target: { value: 'Half-written' },
    })
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))

    expect(onReject).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /approve/i })).toBeInTheDocument()

    // Re-opening shows an empty textarea (the prior input was discarded).
    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    expect(screen.getByPlaceholderText('Rejection reason (required)')).toHaveValue('')
  })

  it('uses the supplied placeholder copy', () => {
    setup({ rejectPlaceholder: 'Be specific to help the contributor learn' })

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))

    expect(
      screen.getByPlaceholderText('Be specific to help the contributor learn')
    ).toBeInTheDocument()
  })

  it('disables every button while a mutation is in flight (isActioning)', () => {
    setup({ isActioning: true })

    expect(screen.getByRole('button', { name: /approve/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /^reject$/i })).toBeDisabled()
  })

  it('shows a spinner on Approve only while isApproving', () => {
    const { container } = render(
      <RejectWithReasonRow
        onApprove={vi.fn()}
        onReject={vi.fn()}
        isActioning
        isApproving
        isRejecting={false}
        rejectPlaceholder="x"
      />
    )

    // Spinner (animate-spin) is present in the resting row while approving.
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows a spinner on Confirm Reject while isRejecting', () => {
    // Expand the row first (resting buttons must be enabled to click Reject),
    // then flip into the in-flight state via rerender — mirrors the real
    // card, where the reject mutation only runs from the expanded view.
    const props = {
      onApprove: vi.fn(),
      onReject: vi.fn(),
      isApproving: false,
      rejectPlaceholder: 'x',
    }
    const { rerender } = render(
      <RejectWithReasonRow {...props} isActioning={false} isRejecting={false} />
    )

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    rerender(<RejectWithReasonRow {...props} isActioning isRejecting />)

    const confirm = screen.getByRole('button', { name: /confirm reject/i })
    expect(confirm.querySelector('.animate-spin')).toBeInTheDocument()
  })

  // PSY-871: the request card relabels the primary action "Create"; default
  // stays "Approve" so the edit/comment cards are unchanged.
  it('renders a custom approve label and fires onApprove', () => {
    const { onApprove } = setup({ approveLabel: 'Create' })

    expect(screen.getByRole('button', { name: /^create$/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^approve$/i })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    expect(onApprove).toHaveBeenCalledTimes(1)
  })
})
