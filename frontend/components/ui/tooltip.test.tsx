import { describe, it, expect, vi } from 'vitest'
import { createRef, type ComponentProps } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from './tooltip'

const TIP = 'Tip content'

function renderTrigger(
  triggerProps: ComponentProps<typeof TooltipTrigger> = {}
) {
  return render(
    <TooltipProvider delayDuration={0}>
      <button type="button">Before</button>
      <Tooltip>
        <TooltipTrigger {...triggerProps}>Trigger</TooltipTrigger>
        <TooltipContent>{TIP}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

const trigger = () => screen.getByRole('button', { name: 'Trigger' })

// Chromium focuses a tapped element after pointerup, so Radix sees a focus
// with no pointer down in progress. This replays that order. fireEvent moves
// no focus of its own, so only the explicit focus() lands.
function tapFocus(element: HTMLElement, pointerType: 'touch' | 'pen') {
  fireEvent.pointerDown(element, { pointerType })
  fireEvent.pointerUp(document, { pointerType })
  act(() => element.focus())
}

// Focus the control before the trigger, then Tab onto the trigger.
async function tabOntoTrigger(user: ReturnType<typeof userEvent.setup>) {
  act(() => screen.getByRole('button', { name: 'Before' }).focus())
  await user.tab()
  expect(trigger()).toHaveFocus()
}

describe('Tooltip', () => {
  it('renders the trigger without throwing', () => {
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent>Tip</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    expect(screen.getByText('Trigger')).toBeInTheDocument()
  })

  it('renders portal content when open', () => {
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent>Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    // Radix renders a visible tooltip plus a visually-hidden a11y copy.
    expect(screen.getAllByText('Tip content').length).toBeGreaterThan(0)
  })

  it('merges a custom className on the content', () => {
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent className="custom-class">Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    const matches = screen.getAllByText('Tip content')
    expect(matches.some(el => el.closest('.custom-class'))).toBe(true)
  })

  it('forwards a ref to the content', () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent ref={ref}>Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})

describe('TooltipTrigger', () => {
  it('forwards a ref to the trigger element', () => {
    const ref = createRef<HTMLButtonElement>()
    renderTrigger({ ref })
    expect(ref.current).toBe(trigger())
  })

  it.each(['touch', 'pen'] as const)(
    'does not open the tooltip on the focus a %s tap gives the trigger',
    pointerType => {
      renderTrigger()

      tapFocus(trigger(), pointerType)

      expect(trigger()).toHaveFocus()
      expect(screen.queryByRole('tooltip')).toBeNull()
      expect(trigger()).toHaveAttribute('data-state', 'closed')
    }
  )

  it('opens the tooltip on keyboard focus', async () => {
    const user = userEvent.setup()
    renderTrigger()

    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('opens the tooltip on mouse hover', async () => {
    const user = userEvent.setup()
    renderTrigger()

    await user.hover(trigger())

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('opens the tooltip on the focus a mouse press gives the trigger', () => {
    renderTrigger()

    fireEvent.pointerDown(trigger(), { pointerType: 'mouse' })
    fireEvent.pointerUp(document, { pointerType: 'mouse' })
    act(() => trigger().focus())

    expect(screen.getByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('suppresses only the one focus that follows a tap', async () => {
    const user = userEvent.setup()
    renderTrigger()

    tapFocus(trigger(), 'touch')
    act(() => trigger().blur())
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('re-enables keyboard focus after a cancelled touch', async () => {
    const user = userEvent.setup()
    renderTrigger()

    // A scroll cancels the touch without a pointerup on the trigger or a
    // focus; a later pointerup ends Radix's own pointer-down tracking.
    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerCancel(trigger(), { pointerType: 'touch' })
    fireEvent.pointerUp(document, { pointerType: 'touch' })
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('re-enables keyboard focus after a tap that never focused the trigger', async () => {
    const user = userEvent.setup()
    renderTrigger()

    // iOS Safari fires the tap's pointer events and click without focusing.
    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerUp(document, { pointerType: 'touch' })
    fireEvent.click(trigger())
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('re-enables keyboard focus after a later mouse press', () => {
    renderTrigger()

    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerUp(document, { pointerType: 'touch' })
    fireEvent.pointerDown(trigger(), { pointerType: 'mouse' })
    fireEvent.pointerUp(document, { pointerType: 'mouse' })
    act(() => trigger().focus())

    expect(screen.getByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('runs the caller handlers it composes with', () => {
    const onPointerDown = vi.fn()
    const onPointerCancel = vi.fn()
    const onFocus = vi.fn()
    const onClick = vi.fn()
    renderTrigger({ onPointerDown, onPointerCancel, onFocus, onClick })

    fireEvent.pointerDown(trigger(), { pointerType: 'touch' })
    fireEvent.pointerCancel(trigger(), { pointerType: 'touch' })
    act(() => trigger().focus())
    fireEvent.click(trigger())

    expect(onPointerDown).toHaveBeenCalledTimes(1)
    expect(onPointerCancel).toHaveBeenCalledTimes(1)
    expect(onFocus).toHaveBeenCalledTimes(1)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('consumes the tap even when the caller default-prevents its focus', async () => {
    const user = userEvent.setup()
    let callerBlocksFocus = true
    renderTrigger({
      onFocus: event => {
        if (callerBlocksFocus) event.preventDefault()
      },
    })

    tapFocus(trigger(), 'touch')
    act(() => trigger().blur())
    callerBlocksFocus = false
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('suppresses the tap focus on an asChild trigger and still runs the child handlers', () => {
    const onChildFocus = vi.fn()
    render(
      <TooltipProvider delayDuration={0}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={0} onFocus={onChildFocus}>
              Span trigger
            </span>
          </TooltipTrigger>
          <TooltipContent>{TIP}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    const span = screen.getByText('Span trigger')

    tapFocus(span, 'touch')

    expect(onChildFocus).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('tooltip')).toBeNull()
  })
})
