import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SubmissionSuccessDialog } from './SubmissionSuccessDialog'

describe('SubmissionSuccessDialog', () => {
  it('renders dialog content when open', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByText('Private Show Added')).toBeInTheDocument()
  })

  it('renders description text about private show', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(
      screen.getByText(/saved to your personal list/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/won't appear in public listings/)
    ).toBeInTheDocument()
  })

  it('renders "Got it" button', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Got it/ })).toBeInTheDocument()
  })

  it('calls onOpenChange with false when "Got it" is clicked', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(<SubmissionSuccessDialog open={true} onOpenChange={onOpenChange} />)

    await user.click(screen.getByRole('button', { name: /Got it/ }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('does not render content when closed', () => {
    render(<SubmissionSuccessDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByText('Private Show Added')).not.toBeInTheDocument()
  })

  it('renders dialog with heading role', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('heading', { name: 'Private Show Added' })).toBeInTheDocument()
  })

  it('renders a dialog element', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  // Focus-management + dismissal coverage (PSY-862): the dialog wraps Radix
  // Dialog primitives, so it inherits the focus-trap + escape-to-close
  // contract. Locking these behaviours into the test ensures that a future
  // primitive swap or wrapping change can't silently regress a11y.

  it('moves focus inside the dialog on open', async () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)

    const dialog = await screen.findByRole('dialog')
    // Radix focuses the first focusable element (or the dialog itself) on
    // open. Either lands inside the dialog tree — we just need to assert
    // that focus did NOT remain on document.body.
    await waitFor(() => {
      expect(dialog.contains(document.activeElement)).toBe(true)
    })
    expect(document.activeElement).not.toBe(document.body)
  })

  it('closes via onOpenChange(false) when Escape is pressed', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(<SubmissionSuccessDialog open={true} onOpenChange={onOpenChange} />)

    // Dialog must be mounted before Escape can reach it.
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.keyboard('{Escape}')

    // Radix Dialog forwards Escape to the controlled onOpenChange; the
    // parent (test) is responsible for actually unmounting the content by
    // re-rendering with open={false}. Assert the contract (callback fires
    // with false) rather than the post-unmount state.
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  it('unmounts content when the parent re-renders with open=false', () => {
    const { rerender } = render(
      <SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />
    )

    expect(screen.getByRole('dialog')).toBeInTheDocument()

    rerender(<SubmissionSuccessDialog open={false} onOpenChange={vi.fn()} />)

    // After the parent flips open→false the Radix portal teardown removes
    // the dialog from the DOM. (Smoke test for the controlled-open
    // contract — guards against a regression where the dialog stops
    // honouring its open prop and "sticks" open.)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('opens when the parent re-renders open=false→true', async () => {
    const onOpenChange = vi.fn()
    const { rerender } = render(
      <SubmissionSuccessDialog open={false} onOpenChange={onOpenChange} />
    )

    // No dialog mounted yet.
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    rerender(<SubmissionSuccessDialog open={true} onOpenChange={onOpenChange} />)

    // Mounting an open Radix dialog is async (portal + focus sequencing).
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Private Show Added')).toBeInTheDocument()
  })

  it('"Got it" button has the leading success icon (Lucide check variant)', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)

    // The button's success affordance is a Lucide check-style icon paired
    // with the "Got it" label. The exact class name has churned across
    // Lucide major versions (`lucide-check-circle-2` → `lucide-circle-check`
    // → `lucide-circle-check-big`), so match the family rather than the
    // exact class. Asserting the icon presence keeps the success-action
    // affordance from being silently demoted (e.g. dropping the icon and
    // shipping a text-only button).
    const button = screen.getByRole('button', { name: /Got it/ })
    const checkIcon = button.querySelector(
      'svg[class*="lucide-check"], svg[class*="lucide-circle-check"]'
    )
    expect(checkIcon).toBeInTheDocument()
  })
})
