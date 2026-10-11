import { describe, it, expect, vi } from 'vitest'
import { createRef, type ComponentProps } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

type PointerType = 'touch' | 'pen' | 'mouse'

function press(element: HTMLElement, pointerType: PointerType) {
  fireEvent.pointerDown(element, { pointerType })
  fireEvent.pointerUp(document, { pointerType })
}

// Chromium focuses a tapped element after pointerup, so Radix sees a focus
// with no pointer down in progress. This replays that order. fireEvent moves
// no focus of its own, so only the explicit focus() lands.
function pressFocus(element: HTMLElement, pointerType: PointerType) {
  press(element, pointerType)
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

      pressFocus(trigger(), pointerType)

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

    pressFocus(trigger(), 'mouse')

    expect(screen.getByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('suppresses only the one focus that follows a tap', async () => {
    const user = userEvent.setup()
    renderTrigger()

    pressFocus(trigger(), 'touch')
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
    press(trigger(), 'touch')
    fireEvent.click(trigger())
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  it('re-enables keyboard focus after a later mouse press', () => {
    renderTrigger()

    press(trigger(), 'touch')
    pressFocus(trigger(), 'mouse')

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

    pressFocus(trigger(), 'touch')
    act(() => trigger().blur())
    callerBlocksFocus = false
    await tabOntoTrigger(user)

    expect(await screen.findByRole('tooltip')).toHaveTextContent(TIP)
  })

  describe('with revealOnTouch', () => {
    function renderRevealing() {
      return render(
        <TooltipProvider delayDuration={0}>
          <p data-testid="outside">Outside</p>
          <Tooltip>
            <TooltipTrigger revealOnTouch>Trigger</TooltipTrigger>
            <TooltipContent>{TIP}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )
    }

    // A whole tap on the trigger: the pointer events, the focus Chromium
    // gives it, then the click that ends the tap.
    function tapTrigger(pointerType: 'touch' | 'pen' = 'touch') {
      pressFocus(trigger(), pointerType)
      fireEvent.click(trigger())
    }

    it.each(['touch', 'pen'] as const)(
      'keeps the tooltip open through a whole %s tap',
      pointerType => {
        renderRevealing()

        tapTrigger(pointerType)

        expect(screen.getByRole('tooltip')).toHaveTextContent(TIP)
      }
    )

    it('still closes the tooltip on a mouse click', async () => {
      const user = userEvent.setup()
      renderRevealing()
      await user.hover(trigger())
      await screen.findByRole('tooltip')

      fireEvent.pointerDown(trigger(), { pointerType: 'mouse' })
      fireEvent.click(trigger())

      expect(screen.queryByRole('tooltip')).toBeNull()
    })

    it('closes the revealed tooltip on the next tap of the trigger', () => {
      renderRevealing()
      tapTrigger()
      expect(screen.getByRole('tooltip')).toBeInTheDocument()

      // The trigger already holds focus, so the second tap brings no focus.
      press(trigger(), 'touch')
      fireEvent.click(trigger())

      expect(screen.queryByRole('tooltip')).toBeNull()
    })

    it('closes the revealed tooltip on a tap outside', async () => {
      renderRevealing()
      tapTrigger()
      expect(screen.getByRole('tooltip')).toBeInTheDocument()

      // Radix registers its outside-press listener a tick after opening, and
      // for a touch it dismisses on the click that ends the tap.
      await act(() => new Promise(resolve => setTimeout(resolve, 0)))
      const outside = screen.getByTestId('outside')
      press(outside, 'touch')
      fireEvent.click(outside)

      await waitFor(() => expect(screen.queryByRole('tooltip')).toBeNull())
    })
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

    pressFocus(span, 'touch')

    expect(onChildFocus).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('tooltip')).toBeNull()
  })
})
