import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import {
  BOTTOM_SHEET_DETENT_HEIGHT_PX,
  BOTTOM_SHEET_DRAG_SLOP_PX,
  BottomSheet,
  bottomSheetHeightCss,
  bottomSheetHeightPx,
  nextGrabberDetent,
  settleBottomSheetDetent,
  type BottomSheetDetent,
} from './bottom-sheet'
import { renderWithProviders, screen } from '@/test/utils'

// The 390x844 Atlas host: 844 minus the 64px top bar and the 57px tab bar.
const HOST = 723
const INSET = 112

describe('bottomSheetHeightPx (the detent rule)', () => {
  it('uses the DS heights when the host has room for them', () => {
    expect(BOTTOM_SHEET_DETENT_HEIGHT_PX).toEqual({ peek: 120, half: 400, full: 660 })
    expect(bottomSheetHeightPx('peek', 900, 0)).toBe(120)
    expect(bottomSheetHeightPx('half', 900, 0)).toBe(400)
    expect(bottomSheetHeightPx('full', 900, 0)).toBe(660)
  })

  it('caps every detent at the host height minus the top inset', () => {
    expect(bottomSheetHeightPx('full', HOST, INSET)).toBe(HOST - INSET)
    expect(bottomSheetHeightPx('half', HOST, INSET)).toBe(400)
    // 390x664: the host is 543px, so Half still fits but Full is capped.
    expect(bottomSheetHeightPx('half', 543, INSET)).toBe(400)
    expect(bottomSheetHeightPx('full', 543, INSET)).toBe(543 - INSET)
    // A host shorter than the inset collapses to zero rather than going negative.
    expect(bottomSheetHeightPx('peek', 50, INSET)).toBe(0)
  })

  it('states the same rule in CSS against the host height', () => {
    expect(bottomSheetHeightCss('half', INSET)).toBe('min(400px, calc(100% - 112px))')
  })
})

describe('settleBottomSheetDetent', () => {
  const settle = (heightPx: number, velocityPxPerMs = 0) =>
    settleBottomSheetDetent({ heightPx, velocityPxPerMs, hostHeightPx: HOST, topInsetPx: INSET })

  it('snaps a slow release to the nearest detent', () => {
    expect(settle(150)).toBe('peek')
    expect(settle(300)).toBe('half')
    expect(settle(520)).toBe('full')
  })

  it('carries a fling to the next detent in its direction', () => {
    // Released just above Peek, but moving up fast: Half, not Peek.
    expect(settle(130, 0.8)).toBe('half')
    // Released just under Full, moving down fast: Half.
    expect(settle(600, -0.8)).toBe('half')
    // Past the last detent in the fling direction: the end detent.
    expect(settle(HOST - INSET, 2)).toBe('full')
    expect(settle(120, -2)).toBe('peek')
  })
})

describe('nextGrabberDetent', () => {
  it('steps up through detents and wraps from the tallest to Peek', () => {
    expect(nextGrabberDetent('peek', HOST, INSET)).toBe('half')
    expect(nextGrabberDetent('half', HOST, INSET)).toBe('full')
    expect(nextGrabberDetent('full', HOST, INSET)).toBe('peek')
  })
  it('skips a detent the host caps to the same height', () => {
    // A 400px host leaves 288px: Half and Full both render at 288.
    expect(nextGrabberDetent('half', 400, INSET)).toBe('peek')
    expect(nextGrabberDetent('peek', 400, INSET)).toBe('half')
  })
  it('walks the plain order on an unmeasured host', () => {
    expect(nextGrabberDetent('half', 0, INSET)).toBe('full')
  })
})

function Harness({
  onClose,
  onDismiss,
  onDetentChange,
  defaultDetent,
}: {
  onClose?: () => void
  onDismiss?: () => void
  onDetentChange?: (d: BottomSheetDetent) => void
  defaultDetent?: BottomSheetDetent
}) {
  return (
    <div style={{ position: 'relative' }}>
      <button type="button">Map control</button>
      <BottomSheet
        title="Chicago, IL"
        label="Chicago, IL scene"
        onClose={onClose}
        onDismiss={onDismiss}
        onDetentChange={onDetentChange}
        defaultDetent={defaultDetent}
        topInsetPx={INSET}
        data-testid="sheet"
      >
        <button type="button">Open scene</button>
      </BottomSheet>
    </div>
  )
}

/** A touch pointer event with an explicit timestamp (jsdom's is the wall clock). */
function pointer(
  type: string,
  init: { clientY: number; timeStamp: number; button?: number },
): Event {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    clientY: init.clientY,
    button: init.button ?? 0,
  })
  Object.defineProperties(event, {
    pointerId: { value: 1 },
    pointerType: { value: 'touch' },
    timeStamp: { value: init.timeStamp },
  })
  return event
}

/** jsdom has no layout: give the sheet and its host the geometry a phone has. */
function stubGeometry(sheet: HTMLElement, sheetHeight: number) {
  sheet.getBoundingClientRect = () =>
    ({ height: sheetHeight, top: HOST - sheetHeight, bottom: HOST }) as DOMRect
  Object.defineProperty(sheet.parentElement!, 'clientHeight', {
    configurable: true,
    value: HOST,
  })
}

describe('BottomSheet', () => {
  it('is non-modal: a labelled region with no aria-modal and no dialog role', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByRole('region', { name: 'Chicago, IL' })
    expect(sheet).not.toHaveAttribute('aria-modal')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    // Content behind the sheet stays reachable.
    expect(screen.getByRole('button', { name: 'Map control' })).toBeVisible()
  })

  it('renders at its default detent, sized by the detent rule', () => {
    renderWithProviders(<Harness onClose={vi.fn()} defaultDetent="half" />)
    const sheet = screen.getByTestId('sheet')
    expect(sheet).toHaveAttribute('data-detent', 'half')
    expect(sheet.style.getPropertyValue('--bottom-sheet-height')).toBe(
      bottomSheetHeightCss('half', INSET),
    )
  })

  it('animates detent changes only without reduced motion', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    expect(screen.getByTestId('sheet').className).toContain(
      'motion-reduce:transition-none',
    )
  })

  it('closes from the 24px close control', async () => {
    const onClose = vi.fn()
    renderWithProviders(<Harness onClose={onClose} />)
    const close = screen.getByRole('button', { name: 'Close Chicago, IL scene' })
    expect(close.className).toContain('size-6')
    await userEvent.click(close)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('renders no close control without onClose', () => {
    renderWithProviders(<Harness />)
    expect(screen.queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
  })

  it('closes on Escape, and prefers onDismiss when given', async () => {
    const onClose = vi.fn()
    const { unmount } = renderWithProviders(<Harness onClose={onClose} />)
    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
    unmount()

    const onDismiss = vi.fn()
    const onClose2 = vi.fn()
    renderWithProviders(<Harness onClose={onClose2} onDismiss={onDismiss} />)
    await userEvent.keyboard('{Escape}')
    expect(onDismiss).toHaveBeenCalledTimes(1)
    expect(onClose2).not.toHaveBeenCalled()
  })

  it('does not close on a press outside it', async () => {
    const onClose = vi.fn()
    renderWithProviders(<Harness onClose={onClose} />)
    await userEvent.click(screen.getByRole('button', { name: 'Map control' }))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('reaches the grabber, the close control and the body by Tab', async () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    await userEvent.tab() // the map control behind the sheet
    await userEvent.tab()
    expect(screen.getByRole('button', { name: 'Expand Chicago, IL scene' })).toHaveFocus()
    await userEvent.tab()
    expect(screen.getByRole('button', { name: 'Close Chicago, IL scene' })).toHaveFocus()
    await userEvent.tab()
    expect(screen.getByRole('button', { name: 'Open scene' })).toHaveFocus()
  })

  it('steps up one detent per grabber tap, and from Full back to Peek', async () => {
    const onDetentChange = vi.fn()
    renderWithProviders(<Harness onClose={vi.fn()} onDetentChange={onDetentChange} />)
    const sheet = screen.getByTestId('sheet')
    await userEvent.click(screen.getByRole('button', { name: 'Expand Chicago, IL scene' }))
    expect(sheet).toHaveAttribute('data-detent', 'half')
    await userEvent.click(screen.getByRole('button', { name: 'Expand Chicago, IL scene' }))
    expect(sheet).toHaveAttribute('data-detent', 'full')
    await userEvent.click(screen.getByRole('button', { name: 'Collapse Chicago, IL scene' }))
    expect(sheet).toHaveAttribute('data-detent', 'peek')
    expect(onDetentChange.mock.calls.map((c) => c[0])).toEqual(['half', 'full', 'peek'])
  })

  it('follows a drag on the handle and settles on the nearest detent', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')

    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })
    // Mid-drag the sheet tracks the finger: 120 + 200 of travel.
    expect(sheet).toHaveAttribute('data-dragging', 'true')
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('320px')
    fireEvent.pointerUp(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })

    expect(sheet).not.toHaveAttribute('data-dragging')
    // The live height is cleared, so the settled detent's height applies.
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('')
    expect(sheet).toHaveAttribute('data-detent', 'half')
  })

  it('caps a drag at the Full height for the host', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 700, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 0, pointerType: 'touch' })
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe(`${HOST - INSET}px`)
  })

  it('treats travel under the slop as a tap, not a drag', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, {
      pointerId: 1,
      clientY: 600 - (BOTTOM_SHEET_DRAG_SLOP_PX - 1),
      pointerType: 'touch',
    })
    fireEvent.pointerUp(handle, { pointerId: 1, clientY: 597, pointerType: 'touch' })
    expect(sheet).not.toHaveAttribute('data-dragging')
    expect(sheet).toHaveAttribute('data-detent', 'peek')
  })

  it('does not start a drag from the close control', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const close = screen.getByRole('button', { name: 'Close Chicago, IL scene' })
    fireEvent.pointerDown(close, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(close, { pointerId: 1, clientY: 300, pointerType: 'touch' })
    expect(sheet).not.toHaveAttribute('data-dragging')
  })

  it('ignores the click a drag released over the grabber ends in', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const grabber = screen.getByRole('button', { name: 'Expand Chicago, IL scene' })
    fireEvent.pointerDown(grabber, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(grabber, { pointerId: 1, clientY: 590, pointerType: 'touch' })
    fireEvent.pointerUp(grabber, { pointerId: 1, clientY: 590, pointerType: 'touch' })
    // Whatever the drag settled on, the click that trails it changes nothing.
    const settled = sheet.getAttribute('data-detent')
    fireEvent.click(grabber)
    expect(sheet).toHaveAttribute('data-detent', settled!)
  })

  it('drops a cancelled drag and keeps the detent', () => {
    renderWithProviders(<Harness onClose={vi.fn()} defaultDetent="half" />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 400)
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 400, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 200, pointerType: 'touch' })
    expect(sheet).toHaveAttribute('data-dragging', 'true')
    fireEvent.pointerCancel(handle, { pointerId: 1, pointerType: 'touch' })
    expect(sheet).not.toHaveAttribute('data-dragging')
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('')
    expect(sheet).toHaveAttribute('data-detent', 'half')
  })

  it('leaves Escape typed into a field to the field', () => {
    const onClose = vi.fn()
    renderWithProviders(
      <div style={{ position: 'relative' }}>
        <BottomSheet title="T" label="list" onClose={onClose}>
          <select aria-label="Genre">
            <option>All</option>
          </select>
        </BottomSheet>
      </div>,
    )
    fireEvent.keyDown(screen.getByRole('combobox', { name: 'Genre' }), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('gives the grabber at least a 24px-tall target', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    expect(
      screen.getByRole('button', { name: 'Expand Chicago, IL scene' }).className,
    ).toContain('h-6')
  })

  it('keeps a touch drag alive when a child hands its capture to the handle', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const grabber = screen.getByRole('button', { name: 'Expand Chicago, IL scene' })
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(grabber, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(grabber, { pointerId: 1, clientY: 580, pointerType: 'touch' })
    // The grabber's implicit capture moves to the handle; its loss bubbles.
    fireEvent.lostPointerCapture(grabber, { pointerId: 1, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })
    expect(sheet).toHaveAttribute('data-dragging', 'true')
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('320px')
    // The handle itself losing the pointer does end it.
    fireEvent.lostPointerCapture(handle, { pointerId: 1, pointerType: 'touch' })
    expect(sheet).not.toHaveAttribute('data-dragging')
  })

  it('settles a fast drag held still before release by position, not fling', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    // 330px up in 10ms is a fling, but the finger then rests 500ms.
    fireEvent(handle, pointer('pointerdown', { clientY: 600, timeStamp: 1000, button: 0 }))
    fireEvent(handle, pointer('pointermove', { clientY: 450, timeStamp: 1005 }))
    fireEvent(handle, pointer('pointermove', { clientY: 270, timeStamp: 1010 }))
    fireEvent(handle, pointer('pointerup', { clientY: 270, timeStamp: 1510 }))
    // Released at 450px: nearest is Half, where a fling would have gone Full.
    expect(sheet).toHaveAttribute('data-detent', 'half')
  })

  it('moves focus from the body to the grabber on Escape', async () => {
    const onDismiss = vi.fn()
    renderWithProviders(
      <div style={{ position: 'relative' }}>
        <BottomSheet title="T" label="list" onDismiss={onDismiss} defaultDetent="half">
          <button type="button">Row</button>
        </BottomSheet>
      </div>,
    )
    screen.getByRole('button', { name: 'Row' }).focus()
    await userEvent.keyboard('{Escape}')
    expect(onDismiss).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: /list$/ })).toHaveFocus()
  })

  it('ignores a second pointer while a drag is live', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 600, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })
    fireEvent.pointerDown(handle, { pointerId: 2, clientY: 500, button: 0, pointerType: 'touch' })
    fireEvent.pointerUp(handle, { pointerId: 2, clientY: 500, pointerType: 'touch' })
    // The first finger still owns the drag and its release settles it.
    expect(sheet).toHaveAttribute('data-dragging', 'true')
    fireEvent.pointerUp(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })
    expect(sheet).not.toHaveAttribute('data-dragging')
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('')
  })

  it('treats a buttonless mouse move as a hover, never a drag', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    // Pressed here, released elsewhere: no pointerup reaches the handle.
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 600, button: 0, buttons: 1, pointerType: 'mouse' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 300, buttons: 0, pointerType: 'mouse' })
    expect(sheet).not.toHaveAttribute('data-dragging')
    expect(sheet.style.getPropertyValue('--bottom-sheet-drag-height')).toBe('')
  })

  it('labels the grabber Collapse when its tap would shrink the sheet', () => {
    const observers: (() => void)[] = []
    const Original = window.ResizeObserver
    window.ResizeObserver = class {
      constructor(private cb: () => void) {
        observers.push(() => this.cb())
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver
    try {
      renderWithProviders(<Harness onClose={vi.fn()} defaultDetent="half" />)
      const sheet = screen.getByTestId('sheet')
      // A 400px host caps Half and Full to one height: the tap goes to Peek.
      Object.defineProperty(sheet.parentElement!, 'clientHeight', {
        configurable: true,
        value: 400,
      })
      act(() => observers.forEach((o) => o()))
      const grabber = screen.getByRole('button', { name: 'Collapse Chicago, IL scene' })
      fireEvent.click(grabber)
      expect(sheet).toHaveAttribute('data-detent', 'peek')
    } finally {
      window.ResizeObserver = Original
    }
  })

  it('honours a controlled detent', async () => {
    function Controlled() {
      const [detent, setDetent] = useState<BottomSheetDetent>('full')
      return (
        <div style={{ position: 'relative' }}>
          <BottomSheet title="T" label="list" detent={detent} onDetentChange={setDetent} data-testid="sheet">
            body
          </BottomSheet>
          <button type="button" onClick={() => setDetent('peek')}>
            Collapse from outside
          </button>
        </div>
      )
    }
    renderWithProviders(<Controlled />)
    expect(screen.getByTestId('sheet')).toHaveAttribute('data-detent', 'full')
    await userEvent.click(screen.getByRole('button', { name: 'Collapse from outside' }))
    expect(screen.getByTestId('sheet')).toHaveAttribute('data-detent', 'peek')
  })
})
