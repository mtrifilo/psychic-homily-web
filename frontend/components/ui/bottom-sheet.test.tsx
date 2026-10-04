import { describe, it, expect, vi } from 'vitest'
import { fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import {
  BOTTOM_SHEET_DETENT_HEIGHT_PX,
  BOTTOM_SHEET_DRAG_SLOP_PX,
  BottomSheet,
  bottomSheetHeightCss,
  bottomSheetHeightPx,
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
    expect(sheet.style.getPropertyValue('--bottom-sheet-height')).toBe('320px')
    fireEvent.pointerUp(handle, { pointerId: 1, clientY: 400, pointerType: 'touch' })

    expect(sheet).not.toHaveAttribute('data-dragging')
    expect(sheet).toHaveAttribute('data-detent', 'half')
  })

  it('caps a drag at the Full height for the host', () => {
    renderWithProviders(<Harness onClose={vi.fn()} />)
    const sheet = screen.getByTestId('sheet')
    stubGeometry(sheet, 120)
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientY: 700, button: 0, pointerType: 'touch' })
    fireEvent.pointerMove(handle, { pointerId: 1, clientY: 0, pointerType: 'touch' })
    expect(sheet.style.getPropertyValue('--bottom-sheet-height')).toBe(`${HOST - INSET}px`)
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
