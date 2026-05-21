import { describe, it, expect } from 'vitest'
import userEvent from '@testing-library/user-event'

import {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from './sheet'
import { renderWithProviders, screen, waitFor } from '@/test/utils'

type SheetSide = 'top' | 'right' | 'bottom' | 'left'

/**
 * Small harness: a trigger plus sheet content with two focusable
 * children, mirroring the dialog harness. `side` and `modal` are
 * forwarded so individual tests can vary them.
 *
 * As with Dialog, `modal={false}` only stops freezing outside
 * pointer-events in this version of Radix — it does not disable
 * outside-click dismissal.
 */
function SheetHarness({ side, modal }: { side?: SheetSide; modal?: boolean }) {
  return (
    <Sheet modal={modal}>
      <SheetTrigger>Open sheet</SheetTrigger>
      <SheetContent side={side}>
        <SheetHeader>
          <SheetTitle>Sheet title</SheetTitle>
          <SheetDescription>Sheet description</SheetDescription>
        </SheetHeader>
        <input type="text" aria-label="first field" />
        <button type="button">Confirm</button>
      </SheetContent>
    </Sheet>
  )
}

// Sheet (built on Radix Dialog) reuses the dialog role for its content,
// so role="dialog" is the canonical query for its content element.
//
// SheetOverlay renders as a fixed inset-0 sibling and is the element a
// real outside-click lands on. We click it directly because Radix sets
// `pointer-events: none` on <body> while a modal sheet is open, so
// clicking document.body throws under jsdom rather than dismissing — a
// documented limitation of exercising Radix's document-level pointerdown
// dismissal in this environment.
function getOverlay(): HTMLElement {
  const overlay = document.querySelector<HTMLElement>(
    '[data-slot="sheet-overlay"]'
  )
  if (!overlay) throw new Error('sheet overlay not found')
  return overlay
}

describe('Sheet', () => {
  it('does not render content until the trigger is activated', () => {
    renderWithProviders(<SheetHarness />)

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText('Open sheet')).toBeInTheDocument()
  })

  it('opens and renders content with role="dialog" and an accessible name', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))

    const sheet = await screen.findByRole('dialog')
    expect(sheet).toBeInTheDocument()
    expect(sheet).toHaveAttribute('data-state', 'open')
    expect(sheet).toHaveAttribute('data-slot', 'sheet-content')
    // This version of Radix conveys modality via role="dialog" plus
    // aria-labelledby/aria-describedby, NOT an `aria-modal` attribute.
    expect(sheet).toHaveAttribute('aria-labelledby')
    expect(sheet).toHaveAttribute('aria-describedby')
    expect(sheet).toHaveAccessibleName('Sheet title')
  })

  it('moves focus inside the sheet on open', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))

    const sheet = await screen.findByRole('dialog')
    await waitFor(() => {
      expect(sheet.contains(document.activeElement)).toBe(true)
    })
  })

  it('traps focus: Tab cycles within the sheet', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))

    const sheet = await screen.findByRole('dialog')
    await waitFor(() => {
      expect(sheet.contains(document.activeElement)).toBe(true)
    })

    for (let i = 0; i < 6; i++) {
      await user.tab()
      expect(sheet.contains(document.activeElement)).toBe(true)
    }

    await user.tab({ shift: true })
    expect(sheet.contains(document.activeElement)).toBe(true)
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('closes via the built-in Close button', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close' }))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('closes on outside (overlay) click by default', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness />)

    await user.click(screen.getByText('Open sheet'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.click(getOverlay())

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('does not freeze outside pointer-events when modal={false}', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SheetHarness modal={false} />)

    await user.click(screen.getByText('Open sheet'))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    expect(document.body.style.pointerEvents).not.toBe('none')
  })

  describe('side prop', () => {
    // The wrapper does not expose `side` as a data attribute; it maps
    // each side onto the slide-in/out utility classes. Assert on those
    // class tokens so a regression in the mapping is caught.
    const cases: Array<{ side: SheetSide; token: string }> = [
      { side: 'right', token: 'data-[state=open]:slide-in-from-right' },
      { side: 'left', token: 'data-[state=open]:slide-in-from-left' },
      { side: 'top', token: 'data-[state=open]:slide-in-from-top' },
      { side: 'bottom', token: 'data-[state=open]:slide-in-from-bottom' },
    ]

    it.each(cases)(
      'applies the $side slide class ($token)',
      async ({ side, token }) => {
        const user = userEvent.setup()
        renderWithProviders(<SheetHarness side={side} />)

        await user.click(screen.getByText('Open sheet'))

        const sheet = await screen.findByRole('dialog')
        expect(sheet).toHaveClass(token)
      }
    )

    it('defaults to the right side when side is omitted', async () => {
      const user = userEvent.setup()
      renderWithProviders(<SheetHarness />)

      await user.click(screen.getByText('Open sheet'))

      const sheet = await screen.findByRole('dialog')
      expect(sheet).toHaveClass('data-[state=open]:slide-in-from-right')
    })
  })
})
