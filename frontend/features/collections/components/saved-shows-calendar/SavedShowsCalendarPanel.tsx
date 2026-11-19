'use client'

import { useCallback, useId, type RefObject } from 'react'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@/components/ui/popover'
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet'
import { useMediaQuery } from '@/lib/hooks/common/useMediaQuery'
import {
  CalendarPanelLayoutContext,
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
 * server never renders a row and the status is read fresh on every open.
 */
export function SavedShowsCalendarPanel({
  open,
  onOpenChange,
  anchorRef,
  align = 'start',
}: SavedShowsCalendarPanelProps) {
  const fitsPopover = useMediaQuery(POPOVER_MIN_WIDTH_QUERY)
  const headingId = useId()
  const close = useCallback(() => onOpenChange(false), [onOpenChange])

  // The panel has no trigger for the primitives to return focus to.
  const returnFocusToAnchor = useCallback(
    (event: Event) => {
      event.preventDefault()
      anchorRef.current?.querySelector<HTMLElement>('button, a')?.focus()
    },
    [anchorRef]
  )

  if (fitsPopover) {
    return (
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverAnchor
          virtualRef={anchorRef as RefObject<HTMLElement>}
        />
        <PopoverContent
          align={align}
          aria-labelledby={headingId}
          onCloseAutoFocus={returnFocusToAnchor}
          className="w-72 p-4 shadow-md"
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
            hasToken={state.hasToken}
            onNavigate={onNavigate}
          />
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
  if (state.kind === 'loading') {
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
