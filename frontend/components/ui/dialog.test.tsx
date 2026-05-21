import { describe, it, expect } from 'vitest'
import userEvent from '@testing-library/user-event'

import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from './dialog'
import { renderWithProviders, screen, waitFor } from '@/test/utils'

/**
 * Small harness: a trigger plus dialog content with two focusable
 * children, so we can exercise the focus trap (Tab should cycle within
 * the dialog rather than escaping to document body).
 *
 * `modal` is forwarded to the Radix Root (the wrapper does not expose it
 * on DialogContent). Note that in this version of @radix-ui/react-dialog,
 * `modal={false}` only stops freezing outside pointer-events — it does
 * NOT disable outside-click dismissal. See the modal tests below.
 */
function DialogHarness({ modal }: { modal?: boolean }) {
  return (
    <Dialog modal={modal}>
      <DialogTrigger>Open dialog</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Dialog title</DialogTitle>
          <DialogDescription>Dialog description</DialogDescription>
        </DialogHeader>
        <input type="text" aria-label="first field" />
        <button type="button">Confirm</button>
      </DialogContent>
    </Dialog>
  )
}

// The shadcn DialogContent renders DialogOverlay as a fixed inset-0 sibling.
// It is the element a real outside-click lands on. We click it directly
// because Radix sets `pointer-events: none` on <body> while a modal dialog
// is open, so `userEvent.click(document.body)` throws under jsdom rather
// than dismissing — a documented limitation of exercising Radix's
// document-level pointerdown dismissal in this environment.
function getOverlay(): HTMLElement {
  const overlay = document.querySelector<HTMLElement>('.fixed.inset-0')
  if (!overlay) throw new Error('dialog overlay not found')
  return overlay
}

describe('Dialog', () => {
  it('does not render content until the trigger is activated', () => {
    renderWithProviders(<DialogHarness />)

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText('Open dialog')).toBeInTheDocument()
  })

  it('opens and renders content with role="dialog" and an accessible name', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toBeInTheDocument()
    expect(dialog).toHaveAttribute('data-state', 'open')
    // This version of Radix conveys modality via role="dialog" plus
    // aria-labelledby/aria-describedby (and aria-hidden siblings), NOT an
    // `aria-modal` attribute, so we assert the label association instead.
    expect(dialog).toHaveAttribute('aria-labelledby')
    expect(dialog).toHaveAttribute('aria-describedby')
    expect(dialog).toHaveAccessibleName('Dialog title')
  })

  it('moves focus inside the dialog on open', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))

    const dialog = await screen.findByRole('dialog')
    // Focus should land somewhere inside the dialog, not back on the
    // trigger or on document.body.
    await waitFor(() => {
      expect(dialog.contains(document.activeElement)).toBe(true)
    })
  })

  it('traps focus: Tab cycles within the dialog', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))

    const dialog = await screen.findByRole('dialog')
    await waitFor(() => {
      expect(dialog.contains(document.activeElement)).toBe(true)
    })

    // Tab several times; focus must remain inside the dialog the whole
    // time (the trap prevents it from reaching the document body).
    for (let i = 0; i < 6; i++) {
      await user.tab()
      expect(dialog.contains(document.activeElement)).toBe(true)
    }

    // Shift+Tab backwards stays inside the trap too.
    await user.tab({ shift: true })
    expect(dialog.contains(document.activeElement)).toBe(true)
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('closes via the built-in Close button', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close' }))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('closes on outside (overlay) click by default', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.click(getOverlay())

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('freezes outside pointer-events when modal (default)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    // A modal dialog disables interaction with the rest of the page.
    expect(document.body.style.pointerEvents).toBe('none')
  })

  it('does not freeze outside pointer-events when modal={false}', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness modal={false} />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    // The page behind a non-modal dialog stays interactive. (Radix still
    // closes a non-modal dialog on outside click; `modal` controls
    // pointer-event freezing, not dismissal.)
    expect(document.body.style.pointerEvents).not.toBe('none')
  })

  it('closes on Escape when modal={false}', async () => {
    const user = userEvent.setup()
    renderWithProviders(<DialogHarness modal={false} />)

    await user.click(screen.getByText('Open dialog'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })
})
