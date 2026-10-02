import { afterEach, describe, it, expect, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog'

import { InfoTooltip } from './InfoTooltip'

const COPY = 'Controls immediate alerts when a new show is added.'
const LABEL = 'What do these alerts control?'

function renderInfo(props: Partial<Parameters<typeof InfoTooltip>[0]> = {}) {
  return render(
    <div>
      <p data-testid="outside">Outside the explainer</p>
      <InfoTooltip copy={COPY} label={LABEL} testId="info-glyph" {...props} />
    </div>
  )
}

const trigger = () => screen.getByRole('button', { name: LABEL })

// Comfortably past InfoTooltip's 120 ms tooltip delay, so an absence check
// gives a regression time to render.
const PAST_HOVER_DELAY_MS = 300

// Chromium focuses a tapped button after pointerup, so Radix sees a focus
// with no pointer down in progress. This replays that order.
function tapFocus(button: HTMLElement) {
  fireEvent.pointerDown(button, { pointerType: 'touch' })
  fireEvent.pointerMove(button, { pointerType: 'touch' })
  fireEvent.pointerUp(document, { pointerType: 'touch' })
  act(() => button.focus())
}

describe('InfoTooltip', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('passes the testId through to the trigger button', () => {
    renderInfo()
    expect(screen.getByTestId('info-glyph')).toBe(trigger())
  })

  it('starts collapsed with no explainer rendered', () => {
    renderInfo()
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('opens the popover with the copy on click and sets aria-expanded', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    expect(trigger()).toHaveAttribute('aria-expanded', 'true')
    expect(trigger()).toHaveAttribute('data-state', 'open')
  })

  it('opens the popover on Enter and on Space from keyboard focus', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.tab()
    expect(trigger()).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    trigger().focus()
    await user.keyboard(' ')
    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
  })

  it('opens the popover from a touch tap', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.pointer({ keys: '[TouchA]', target: trigger() })

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    expect(trigger()).toHaveAttribute('aria-expanded', 'true')
  })

  it('does not flash the tooltip when a tap focuses the trigger', async () => {
    renderInfo()
    const button = trigger()

    tapFocus(button)

    expect(screen.queryByRole('tooltip')).toBeNull()

    fireEvent.click(button)

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('shows the tooltip on keyboard focus', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.tab()

    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
    expect(trigger()).toHaveAccessibleDescription(COPY)
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('shows the tooltip on keyboard focus after an earlier tap', async () => {
    const user = userEvent.setup()
    renderInfo()
    const button = trigger()

    tapFocus(button)
    act(() => button.blur())

    await user.tab()

    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
  })

  it.each([
    ['an outside pointer down', () => screen.getByTestId('outside')],
    ['a second click of the trigger', trigger],
  ])('closes the popover on %s', async (_name, target) => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')
    await user.click(target())

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('closes the popover on Escape', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('describes the trigger with the popover copy while it is open', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')

    expect(trigger()).toHaveAccessibleDescription(COPY)

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).not.toHaveAttribute('aria-describedby')
  })

  it('keeps focus on the trigger when the popover copy is pressed', async () => {
    const user = userEvent.setup()
    render(
      <div>
        <InfoTooltip copy={COPY} label={LABEL} />
        <button type="button">Next control</button>
      </div>
    )

    await user.tab()
    await user.keyboard('{Enter}')
    await user.click(await screen.findByText(COPY))

    expect(trigger()).toHaveFocus()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Next control' })).toHaveFocus()
  })

  it('names the popover after the trigger label', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())

    expect(await screen.findByRole('dialog', { name: LABEL })).toBeInTheDocument()
  })

  it('keeps focus on the trigger and does not reopen the tooltip on Escape', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.tab()
    await user.keyboard('{Enter}')
    await screen.findByRole('dialog')
    expect(trigger()).toHaveFocus()

    await user.keyboard('{Escape}')

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveFocus()
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('lets Tab move past the trigger and closes the popover', async () => {
    const user = userEvent.setup()
    render(
      <div>
        <InfoTooltip copy={COPY} label={LABEL} />
        <button type="button">Next control</button>
      </div>
    )

    await user.tab()
    await user.keyboard('{Enter}')
    await screen.findByRole('dialog')

    await user.tab()

    expect(screen.getByRole('button', { name: 'Next control' })).toHaveFocus()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('shows the hover tooltip when a mouse moves over a tap-focused trigger', async () => {
    renderInfo()
    const button = trigger()

    tapFocus(button)
    fireEvent.pointerMove(button, { pointerType: 'mouse' })

    expect(button).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
  })

  it('closes on Escape without focusing an unfocused trigger or reopening the tooltip', async () => {
    const user = userEvent.setup()
    renderInfo()

    // Safari and Firefox on macOS open the popover without focusing the button.
    fireEvent.click(trigger())
    await screen.findByRole('dialog')
    expect(trigger()).not.toHaveFocus()

    await user.keyboard('{Escape}')

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).not.toHaveFocus()
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('shows the focus tooltip after a tap that never focused the trigger', async () => {
    const user = userEvent.setup()
    render(
      <div>
        <button type="button">Before</button>
        <InfoTooltip copy={COPY} label={LABEL} />
      </div>
    )

    // iOS Safari fires the tap's pointer events and click without focusing.
    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerUp(document, { pointerType: 'touch' })
    fireEvent.click(trigger())
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    act(() => screen.getByRole('button', { name: 'Before' }).focus())
    await user.tab()

    expect(trigger()).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
  })

  it('re-enables the focus tooltip after a cancelled touch', async () => {
    const user = userEvent.setup()
    render(
      <div>
        <button type="button">Before</button>
        <InfoTooltip copy={COPY} label={LABEL} />
      </div>
    )

    // A scroll cancels the touch on the glyph without a pointerup or focus;
    // a later tap elsewhere ends Radix's own pointer-down tracking.
    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerCancel(trigger(), { pointerType: 'touch' })
    fireEvent.pointerUp(document, { pointerType: 'touch' })

    act(() => screen.getByRole('button', { name: 'Before' }).focus())
    await user.tab()

    expect(trigger()).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
  })

  it('closes only the popover on Escape inside a modal dialog', async () => {
    const user = userEvent.setup()
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>Host dialog</DialogTitle>
          <DialogDescription>Hosts the explainer</DialogDescription>
          <InfoTooltip copy={COPY} label={LABEL} />
        </DialogContent>
      </Dialog>
    )

    await user.click(trigger())
    expect(await screen.findByRole('dialog', { name: LABEL })).toHaveTextContent(COPY)

    await user.keyboard('{Escape}')

    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: LABEL })).toBeNull()
    )
    expect(screen.getByRole('dialog', { name: 'Host dialog' })).toBeInTheDocument()
  })

  it('shows the tooltip with the copy on hover without opening the popover', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.hover(trigger())

    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('closes the tooltip when a click opens the popover', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.hover(trigger())
    await screen.findByRole('tooltip')

    await user.click(trigger())

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    await waitFor(() => expect(screen.queryByRole('tooltip')).toBeNull())
  })

  it('holds the tooltip back while the popover is open', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')

    await user.unhover(trigger())
    await user.hover(trigger())

    await act(() => vi.advanceTimersByTimeAsync(PAST_HOVER_DELAY_MS))
    expect(screen.queryByRole('tooltip')).toBeNull()
    expect(screen.getByRole('dialog')).toHaveTextContent(COPY)
  })

  // Safari and Firefox on macOS do not focus a clicked button, so no blur
  // arrives to clear a tooltip request made while the popover was open.
  it('does not pop a stale tooltip after the popover closes', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderInfo()

    fireEvent.click(trigger())
    await screen.findByRole('dialog')
    expect(trigger()).not.toHaveFocus()

    await user.hover(trigger())
    await act(() => vi.advanceTimersByTimeAsync(PAST_HOVER_DELAY_MS))
    await user.unhover(trigger())
    await user.click(screen.getByTestId('outside'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await act(() => vi.advanceTimersByTimeAsync(PAST_HOVER_DELAY_MS))
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  // jsdom applies no CSS, so this pins the classes; the selection behavior
  // itself is checked in a browser.
  it('keeps the hover delay for a hover that follows a closed popover', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderInfo()

    fireEvent.click(trigger())
    await screen.findByRole('dialog')
    await user.hover(trigger())
    await act(() => vi.advanceTimersByTimeAsync(PAST_HOVER_DELAY_MS))
    await user.unhover(trigger())
    await user.click(screen.getByTestId('outside'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.hover(trigger())
    await act(() => vi.advanceTimersByTimeAsync(40))
    expect(screen.queryByRole('tooltip')).toBeNull()
    await act(() => vi.advanceTimersByTimeAsync(PAST_HOVER_DELAY_MS))
    expect(screen.getByRole('tooltip')).toHaveTextContent(COPY)
  })

  it('stops the closing Escape at the popover so outer Escape handlers stay put', async () => {
    const user = userEvent.setup()
    const outerEscape = vi.fn()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') outerEscape()
    }
    document.addEventListener('keydown', onKey)
    try {
      renderInfo()
      await user.click(trigger())
      await screen.findByRole('dialog')

      await user.keyboard('{Escape}')

      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(outerEscape).not.toHaveBeenCalled()
    } finally {
      document.removeEventListener('keydown', onKey)
    }
  })

  it('closes the focus tooltip when Enter opens the popover', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.tab()
    await screen.findByRole('tooltip')
    await user.keyboard('{Enter}')

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    await waitFor(() => expect(screen.queryByRole('tooltip')).toBeNull())
  })

  it('carries the long-press suppression classes on the trigger', () => {
    renderInfo()
    expect(trigger()).toHaveClass('select-none', '[-webkit-touch-callout:none]')
  })
})
