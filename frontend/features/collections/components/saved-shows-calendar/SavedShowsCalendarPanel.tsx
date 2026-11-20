'use client'

import {
  useCallback,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from 'react'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@/components/ui/popover'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'
import {
  CalendarPanelLayoutContext,
  PanelErrorWithRetry,
  PanelNote,
  type CalendarPanelLayout,
} from './CalendarPanelParts'
import { SavedShowsCalendarBody } from './SavedShowsCalendarBody'
import { PANEL_COPY, RESET_SUB } from './savedShowsCalendarCopy'
import {
  useSavedShowsCalendar,
  type SavedShowsCalendarState,
} from './useSavedShowsCalendar'

// Tailwind's `sm`, the breakpoint below which the house swaps a popover for
// a bottom sheet (VenueSortControl, the city filter).
const POPOVER_MIN_WIDTH_QUERY = '(min-width: 640px)'

interface SavedShowsCalendarPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /**
   * The control the panel opens from. The popover is positioned against it,
   * and focus returns to its first button or link when the panel closes.
   */
  anchorRef: RefObject<HTMLElement | null>
  /** Which edge of the anchor the popover lines up with. */
  align?: 'start' | 'end'
}

/**
 * Adds the person's saved shows to their calendar, and reports what their
 * calendar apps have observably done with the link.
 *
 * A popover from `sm` up and a modal bottom sheet below it. Chosen in script
 * rather than by CSS alone because the panel has no trigger of its own to
 * swap: rendering both and hiding one would leave a modal sheet trapping focus
 * behind a hidden overlay. Everything inside mounts only while open, so the
 * server never renders a row. Opening prepares the link (loads it, or
 * creates one when the account has none) before any row is offered, so every
 * row acts inside its own click.
 */
export function SavedShowsCalendarPanel({
  open,
  onOpenChange,
  anchorRef,
  align = 'start',
}: SavedShowsCalendarPanelProps) {
  const fitsPopover = useMediaQuery(POPOVER_MIN_WIDTH_QUERY)
  // The shell is chosen as the panel opens and kept until it opens again:
  // swapping popover and sheet mid-open (a phone rotated across the
  // breakpoint) would remount the contents and lose this open's notes and its
  // reset heading, and swapping as it closes would cut its exit animation.
  // Adjusted during render, the house way to follow a prop without an effect.
  const [shell, setShell] = useState<CalendarPanelLayout>(() =>
    fitsPopover ? 'popover' : 'sheet'
  )
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setShell(fitsPopover ? 'popover' : 'sheet')
  }
  const headingId = useId()
  const close = useCallback(() => onOpenChange(false), [onOpenChange])

  // The panel has no trigger for the primitives to return focus to, so it
  // returns focus to the Save control itself, except after a click or focus
  // move outside: that put focus where the person wanted it, and taking it
  // back would also scroll the page to Save. The same rule Radix applies to a
  // non-modal popover with a trigger.
  const interactedOutside = useRef(false)
  const noteOutsideInteraction = useCallback(() => {
    interactedOutside.current = true
  }, [])
  const forgetOutsideInteraction = useCallback(() => {
    interactedOutside.current = false
  }, [])
  const returnFocusToAnchor = useCallback(
    (event: Event) => {
      event.preventDefault()
      if (!interactedOutside.current) {
        anchorRef.current
          ?.querySelector<HTMLElement>('button, a')
          ?.focus({ preventScroll: true })
      }
      interactedOutside.current = false
    },
    [anchorRef]
  )

  // Tab past the popover's last control (or Shift+Tab before its first)
  // steps out of the panel: it closes and returns focus to Save, rather than
  // looping back into a portaled panel far from the row it belongs to.
  // While no control exists yet (still loading), Tab stays put and only
  // Shift+Tab steps out, since the hint that opened it cannot open it again.
  const closeOnTabOut = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (event.key !== 'Tab') return
      const controls = Array.from(
        event.currentTarget.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled])'
        )
      )
      const edge = event.shiftKey ? controls[0] : controls[controls.length - 1]
      // The panel itself holds focus when it opened before its rows existed;
      // Shift+Tab from there is also a step out.
      if (controls.length === 0 && !event.shiftKey) {
        event.preventDefault()
        return
      }
      const leaving =
        controls.length === 0 ||
        document.activeElement === edge ||
        (event.shiftKey && document.activeElement === event.currentTarget)
      if (leaving) {
        event.preventDefault()
        onOpenChange(false)
      }
    },
    [onOpenChange]
  )

  if (shell === 'popover') {
    return (
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverAnchor
          virtualRef={anchorRef as RefObject<HTMLElement>}
        />
        <PopoverContent
          align={align}
          aria-labelledby={headingId}
          onOpenAutoFocus={forgetOutsideInteraction}
          onInteractOutside={noteOutsideInteraction}
          onCloseAutoFocus={returnFocusToAnchor}
          onKeyDown={closeOnTabOut}
          className="w-72 p-4 shadow-md"
          data-saved-shows-calendar-panel=""
          data-testid="saved-shows-calendar-panel"
        >
          <PanelContents
            layout="popover"
            headingId={headingId}
            onNavigate={close}
          />
        </PopoverContent>
      </Popover>
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="bottom"
        aria-describedby={undefined}
        onCloseAutoFocus={returnFocusToAnchor}
        className="max-h-[85dvh] gap-0 overflow-y-auto rounded-t-lg px-5 pb-6 pt-4"
        data-saved-shows-calendar-panel=""
        data-testid="saved-shows-calendar-panel"
      >
        <PanelContents layout="sheet" headingId={headingId} onNavigate={close} />
      </SheetContent>
    </Sheet>
  )
}

function PanelContents({
  layout,
  headingId,
  onNavigate,
}: {
  layout: CalendarPanelLayout
  headingId: string
  onNavigate: () => void
}) {
  const state = useSavedShowsCalendar()
  const { title, sub } = headingFor(state)

  return (
    <CalendarPanelLayoutContext.Provider value={layout}>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-0.5">
          {layout === 'sheet' ? (
            // Radix names the sheet from its own title id.
            <SheetTitle
              className="pr-8 font-sans text-base leading-[22px]"
            >
              {title}
            </SheetTitle>
          ) : (
            <h2
              id={headingId}
              className="text-sm font-semibold leading-5 text-foreground"
            >
              {title}
            </h2>
          )}
          {sub ? <PanelNote>{sub}</PanelNote> : null}
        </div>
        {state.kind === 'ready' ? (
          <SavedShowsCalendarBody
            model={state.model}
            device={state.device}
            feedUrl={state.feedUrl}
            onNavigate={onNavigate}
          />
        ) : state.kind === 'link_failed' ? (
          <PanelErrorWithRetry onRetry={state.retry}>
            {state.failure === 'create'
              ? PANEL_COPY.createFailed
              : PANEL_COPY.loadFailed}
          </PanelErrorWithRetry>
        ) : (
          <PanelNote>{PANEL_COPY.loading}</PanelNote>
        )}
      </div>
    </CalendarPanelLayoutContext.Provider>
  )
}

function headingFor(state: SavedShowsCalendarState): {
  title: string
  sub: string | null
} {
  if (state.kind !== 'ready') {
    return { title: PANEL_COPY.headingStatus, sub: null }
  }
  const { heading, resetReason } = state.model
  if (resetReason) {
    return { title: PANEL_COPY.headingReset, sub: RESET_SUB[resetReason] }
  }
  if (heading === 'status') {
    return { title: PANEL_COPY.headingStatus, sub: null }
  }
  return { title: PANEL_COPY.headingAdd, sub: PANEL_COPY.subAdd }
}
