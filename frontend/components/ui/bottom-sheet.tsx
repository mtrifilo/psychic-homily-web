'use client'

import {
  useCallback,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type Ref,
} from 'react'
import { DismissableLayer } from '@radix-ui/react-dismissable-layer'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * BottomSheet: the design system's bottom-anchored, NON-modal sheet (DS file
 * `isfHz0oyFK1ALX19IRGg51`, component set `427:24`).
 *
 * It anchors to the bottom of its nearest positioned ancestor (the "host"),
 * never to the viewport, so a host that already stops above fixed page chrome
 * keeps the sheet above that chrome too. There is no scrim, no focus trap and
 * no `aria-modal`: whatever the host draws above the sheet stays interactive.
 * Escape dismisses through Radix's shared layer stack, so a popover opened
 * above the sheet wins Escape first.
 */

export type BottomSheetDetent = 'peek' | 'half' | 'full'

/** Shortest to tallest; settling and the grabber both walk this order. */
const BOTTOM_SHEET_DETENTS: readonly BottomSheetDetent[] = [
  'peek',
  'half',
  'full',
]

/** Each detent's height in CSS px, before the host cap (the DS variants). */
export const BOTTOM_SHEET_DETENT_HEIGHT_PX: Readonly<
  Record<BottomSheetDetent, number>
> = {
  peek: 120,
  half: 400,
  full: 660,
}

/** Pointer travel under which a press on the drag handle is a tap. */
export const BOTTOM_SHEET_DRAG_SLOP_PX = 6

/**
 * Release speed (CSS px per ms) at or above which a drag counts as a fling
 * and moves to the next detent in its direction instead of the nearest one.
 */
const BOTTOM_SHEET_FLING_PX_PER_MS = 0.5

/**
 * The height a detent renders at inside a host of `hostHeightPx`.
 *
 * Rule: the detent's DS height, capped at `hostHeightPx - topInsetPx`. The
 * inset is the strip at the top of the host that no detent may cover, which is
 * how a host keeps its own top chrome visible on a short viewport.
 */
export function bottomSheetHeightPx(
  detent: BottomSheetDetent,
  hostHeightPx: number,
  topInsetPx: number,
): number {
  const cap = Math.max(0, hostHeightPx - topInsetPx)
  return Math.min(BOTTOM_SHEET_DETENT_HEIGHT_PX[detent], cap)
}

/**
 * The same rule as `bottomSheetHeightPx`, as a CSS length resolved against the
 * host's height. Lets the sheet (and anything a host positions above it)
 * render correctly before, or without, a layout measurement.
 */
export function bottomSheetHeightCss(
  detent: BottomSheetDetent,
  topInsetPx: number,
): string {
  return `min(${BOTTOM_SHEET_DETENT_HEIGHT_PX[detent]}px, calc(100% - ${topInsetPx}px))`
}

/**
 * Which detent a drag released at `heightPx` settles on.
 *
 * Below the fling speed it is the detent whose rendered height is nearest the
 * release. At or above it, it is the first detent past the release in the
 * fling's direction (`velocityPxPerMs` is positive when the sheet was growing),
 * or the end detent in that direction when none is past it.
 */
export function settleBottomSheetDetent({
  heightPx,
  velocityPxPerMs,
  hostHeightPx,
  topInsetPx,
}: {
  heightPx: number
  velocityPxPerMs: number
  hostHeightPx: number
  topInsetPx: number
}): BottomSheetDetent {
  const heights = BOTTOM_SHEET_DETENTS.map((d) => ({
    detent: d,
    px: bottomSheetHeightPx(d, hostHeightPx, topInsetPx),
  }))
  if (velocityPxPerMs >= BOTTOM_SHEET_FLING_PX_PER_MS) {
    return (heights.find((h) => h.px > heightPx) ?? heights[heights.length - 1])
      .detent
  }
  if (velocityPxPerMs <= -BOTTOM_SHEET_FLING_PX_PER_MS) {
    return ([...heights].reverse().find((h) => h.px < heightPx) ?? heights[0])
      .detent
  }
  let best = heights[0]
  for (const h of heights) {
    if (Math.abs(h.px - heightPx) < Math.abs(best.px - heightPx)) best = h
  }
  return best.detent
}

/**
 * The grabber's tap target: the next detent that renders taller than this one,
 * and from the tallest back to Peek. Detents the host caps to the same height
 * are skipped, so a tap always moves the sheet. An unmeasured host (height 0)
 * walks the plain order.
 */
export function nextGrabberDetent(
  detent: BottomSheetDetent,
  hostHeightPx: number,
  topInsetPx: number,
): BottomSheetDetent {
  const order = BOTTOM_SHEET_DETENTS
  const from = order.indexOf(detent)
  if (hostHeightPx <= 0) return order[(from + 1) % order.length]
  const current = bottomSheetHeightPx(detent, hostHeightPx, topInsetPx)
  const taller = order
    .slice(from + 1)
    .find((d) => bottomSheetHeightPx(d, hostHeightPx, topInsetPx) > current)
  return taller ?? 'peek'
}

/** The detent's rendered height, as a CSS length against the host. */
export const DETENT_HEIGHT_VAR = '--bottom-sheet-height'
/** The live height during a drag, in px; unset otherwise. */
export const DRAG_HEIGHT_VAR = '--bottom-sheet-drag-height'

// How long after a drag ends a click on the grabber is still the drag's.
const DRAG_CLICK_GUARD_MS = 300

const NO_DRAG_SELECTOR =
  'a, input, select, textarea, button:not([data-bottom-sheet-grabber])'

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.tagName === 'SELECT' ||
    target.isContentEditable
  )
}

interface DragState {
  pointerId: number
  startY: number
  startHeight: number
  hostHeight: number
  lastY: number
  lastTime: number
  velocity: number
  active: boolean
}

export interface BottomSheetProps {
  /** Heading text in the sheet's header. */
  title: ReactNode
  /** Plain-text name for the grabber's accessible label ("Expand <name>"). */
  label: string
  /** Controlled detent. Pair with `onDetentChange`. */
  detent?: BottomSheetDetent
  /** Initial detent when uncontrolled. */
  defaultDetent?: BottomSheetDetent
  onDetentChange?: (detent: BottomSheetDetent) => void
  /** The close control. Without it the header renders no close button. */
  onClose?: () => void
  /** Accessible name of the close control. */
  closeLabel?: string
  /**
   * Escape. Defaults to `onClose`; with neither, the sheet leaves Escape to
   * whatever else handles it.
   */
  onDismiss?: () => void
  /** Strip at the top of the host no detent may cover; see bottomSheetHeightPx. */
  topInsetPx?: number
  children?: ReactNode
  /** Drops the Body slot's DS padding, for content with full-bleed rows. */
  flushBody?: boolean
  className?: string
  /** The sheet's accessible name; defaults to the title. */
  'aria-label'?: string
  'data-testid'?: string
  ref?: Ref<HTMLElement>
  closeRef?: Ref<HTMLButtonElement>
}

export function BottomSheet({
  title,
  label,
  detent: controlledDetent,
  defaultDetent = 'peek',
  onDetentChange,
  onClose,
  closeLabel,
  onDismiss,
  topInsetPx = 0,
  children,
  flushBody = false,
  className,
  'aria-label': ariaLabel,
  'data-testid': testId,
  ref,
  closeRef,
}: BottomSheetProps) {
  const titleId = useId()
  const [uncontrolledDetent, setUncontrolledDetent] =
    useState<BottomSheetDetent>(defaultDetent)
  const detent = controlledDetent ?? uncontrolledDetent
  const setDetent = useCallback(
    (next: BottomSheetDetent) => {
      if (controlledDetent === undefined) setUncontrolledDetent(next)
      onDetentChange?.(next)
    },
    [controlledDetent, onDetentChange],
  )

  const sheetRef = useRef<HTMLElement | null>(null)
  const setSheetRef = useCallback(
    (node: HTMLElement | null) => {
      sheetRef.current = node
      if (typeof ref === 'function') ref(node)
      else if (ref) ref.current = node
    },
    [ref],
  )

  // While a drag is in progress the live height is written straight to
  // DRAG_HEIGHT_VAR on the sheet (one style write per pointer move, no
  // render); the detent's height is the fallback whenever it is unset.
  const [dragging, setDragging] = useState(false)
  const dragRef = useRef<DragState | null>(null)
  const clearDragHeight = () => {
    sheetRef.current?.style.removeProperty(DRAG_HEIGHT_VAR)
    setDragging(false)
  }
  // A drag released over the grabber can end in a click on it; a click that
  // close behind a drag's end belongs to the drag, not to the grabber.
  const lastDragEndRef = useRef(-Infinity)

  const handlePointerCancel = () => {
    dragRef.current = null
    clearDragHeight()
  }

  const handlePointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    // One drag at a time: a second finger never takes over a live drag.
    if (dragRef.current?.active) return
    if (e.pointerType === 'mouse' && e.button !== 0) return
    if ((e.target as Element).closest(NO_DRAG_SELECTOR)) return
    const sheet = sheetRef.current
    if (!sheet) return
    dragRef.current = {
      pointerId: e.pointerId,
      startY: e.clientY,
      startHeight: sheet.getBoundingClientRect().height,
      hostHeight: sheet.parentElement?.clientHeight ?? 0,
      lastY: e.clientY,
      lastTime: e.timeStamp,
      velocity: 0,
      active: false,
    }
  }

  const clampDragHeight = (drag: DragState, clientY: number) => {
    const min = bottomSheetHeightPx('peek', drag.hostHeight, topInsetPx)
    const max = bottomSheetHeightPx('full', drag.hostHeight, topInsetPx)
    return Math.min(max, Math.max(min, drag.startHeight - (clientY - drag.startY)))
  }

  const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== e.pointerId) return
    // A mouse released outside the handle sends no pointerup here; with no
    // button held this is a hover, not a drag.
    if (e.pointerType === 'mouse' && e.buttons === 0) {
      handlePointerCancel()
      return
    }
    if (!drag.active) {
      if (Math.abs(e.clientY - drag.startY) < BOTTOM_SHEET_DRAG_SLOP_PX) return
      drag.active = true
      setDragging(true)
      // Captured only once the press is a drag: capturing on press would
      // retarget the click a tap on the grabber relies on.
      e.currentTarget.setPointerCapture?.(e.pointerId)
    }
    const elapsed = e.timeStamp - drag.lastTime
    if (elapsed > 0) drag.velocity = (drag.lastY - e.clientY) / elapsed
    drag.lastY = e.clientY
    drag.lastTime = e.timeStamp
    sheetRef.current?.style.setProperty(
      DRAG_HEIGHT_VAR,
      `${clampDragHeight(drag, e.clientY)}px`,
    )
  }

  const handlePointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== e.pointerId) return
    dragRef.current = null
    if (!drag.active) {
      clearDragHeight()
      return
    }
    lastDragEndRef.current = performance.now()
    clearDragHeight()
    setDetent(
      settleBottomSheetDetent({
        heightPx: clampDragHeight(drag, e.clientY),
        velocityPxPerMs: drag.velocity,
        hostHeightPx: drag.hostHeight,
        topInsetPx,
      }),
    )
  }


  const handleGrabberClick = () => {
    if (performance.now() - lastDragEndRef.current < DRAG_CLICK_GUARD_MS) return
    const hostHeight = sheetRef.current?.parentElement?.clientHeight ?? 0
    setDetent(nextGrabberDetent(detent, hostHeight, topInsetPx))
  }

  const dismiss = onDismiss ?? onClose
  const grabberAction = detent === 'full' ? 'Collapse' : 'Expand'

  return (
    <DismissableLayer
      asChild
      onEscapeKeyDown={(e) => {
        // Escape typed into a field belongs to that field.
        if (isEditableTarget(e.target)) e.preventDefault()
      }}
      onDismiss={dismiss}
      // Non-modal: the host behind the sheet stays usable, so neither a press
      // nor focus outside the sheet may dismiss it.
      onPointerDownOutside={(e) => e.preventDefault()}
      onFocusOutside={(e) => e.preventDefault()}
    >
      <section
        ref={setSheetRef}
        aria-label={ariaLabel}
        aria-labelledby={ariaLabel ? undefined : titleId}
        data-testid={testId}
        data-slot="bottom-sheet"
        data-detent={detent}
        data-dragging={dragging ? 'true' : undefined}
        style={
          {
            // Custom properties rather than `height` directly so each value
            // reads back as authored.
            [DETENT_HEIGHT_VAR]: bottomSheetHeightCss(detent, topInsetPx),
            height: `var(${DRAG_HEIGHT_VAR}, var(${DETENT_HEIGHT_VAR}))`,
          } as CSSProperties
        }
        className={cn(
          'absolute inset-x-0 bottom-0 z-20 flex flex-col overflow-hidden rounded-t-lg border-t border-border bg-popover text-popover-foreground shadow-lg',
          !dragging &&
            'transition-[height] duration-200 ease-out motion-reduce:transition-none',
          className,
        )}
      >
        <div
          data-testid="bottom-sheet-handle"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerCancel={handlePointerCancel}
          onLostPointerCapture={handlePointerCancel}
          // The handle owns vertical drags; the browser must not turn them
          // into a page scroll or a pinch.
          className="shrink-0 touch-none select-none"
        >
          <div className="flex justify-center py-0.5">
            <button
              type="button"
              data-bottom-sheet-grabber=""
              onClick={handleGrabberClick}
              aria-label={`${grabberAction} ${label}`}
              className="flex h-6 w-12 cursor-grab items-center justify-center rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
            >
              <span
                aria-hidden="true"
                className="h-1 w-9 rounded-[2px] bg-border"
              />
            </button>
          </div>
          <div className="flex items-center gap-3 pb-2 pl-4 pr-3 pt-1">
            <h2
              id={titleId}
              className="min-w-0 flex-1 truncate text-base font-bold leading-tight text-foreground"
            >
              {title}
            </h2>
            {onClose && (
              <button
                ref={closeRef}
                type="button"
                onClick={onClose}
                aria-label={closeLabel ?? `Close ${label}`}
                className="flex size-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <X className="size-3" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
        <div
          data-testid="bottom-sheet-body"
          // overscroll-contain: reaching the end of the body never chains the
          // scroll to the page behind the sheet.
          className={cn(
            'min-h-0 flex-1 overflow-y-auto overscroll-contain',
            !flushBody && 'px-4 pb-4 pt-1',
          )}
        >
          {children}
        </div>
      </section>
    </DismissableLayer>
  )
}
