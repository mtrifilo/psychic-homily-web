'use client'

import { useCallback, useEffect, useLayoutEffect, type RefObject } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'
import {
  autoUpdate,
  flip,
  offset,
  shift,
  useFloating,
} from '@floating-ui/react-dom'
import { useDismissFirstSaveHint } from '@/features/shows/hooks/useFirstSaveHint'

// Minimum distance between the hint and either edge of the viewport. The
// hint's max width below (100vw - 2rem) is this gutter on both sides.
const VIEWPORT_GUTTER_PX = 16

/** Height of the fixed mobile tab bar, or 0 where it is not rendered (it is
 *  display:none from `xl`, which measures as 0). Read at each positioning so
 *  a viewport resize across `xl` is honoured. */
function bottomTabBarHeight(): number {
  const bar = document.querySelector<HTMLElement>('[data-bottom-tab-bar]')
  return bar?.getBoundingClientRect().height ?? 0
}

interface FirstSaveHintProps {
  /** The Save control's wrapper. The hint positions against it; when focus
   *  is inside the hint as it closes, focus moves to the first button or link
   *  in this wrapper, which is the Save control. */
  anchorRef: RefObject<HTMLElement | null>
  /** Which edge of the Save control the hint lines up with. */
  align: 'start' | 'end'
  /** Close the hint locally. The hint stamps the account itself. */
  onClose: () => void
}

/**
 * The one-time note that follows a viewer's first saved show, naming where the
 * save went.
 *
 * Rendered in place, right after the Save control, so it is the next stop in
 * tab and reading order; `position: fixed` (anchored by floating-ui) is what
 * lets it overlay what follows without pushing a dense row apart and without
 * being cut off by an `overflow-hidden` or scrolling ancestor. It is not a
 * dismissable layer: a menu or popover opened near it keeps its own Escape and
 * outside clicks. Each of its own controls (Dismiss, either link, Escape)
 * stamps the account. Unsaving the show, or the control unmounting, closes it
 * without stamping.
 */
export function FirstSaveHint({ anchorRef, align, onClose }: FirstSaveHintProps) {
  const { mutate: dismissOnAccount } = useDismissFirstSaveHint()
  const {
    floatingStyles,
    isPositioned,
    placement,
    refs: { setReference, setFloating, floating: hintRef },
  } = useFloating({
    strategy: 'fixed',
    placement: align === 'start' ? 'bottom-start' : 'bottom-end',
    // flip moves it above the control when there is no room below, counting
    // the fixed mobile tab bar as unavailable space; it keeps the requested
    // edge. shift keeps it inside the viewport gutter.
    middleware: [
      offset(8),
      flip(() => ({
        padding: {
          top: VIEWPORT_GUTTER_PX,
          right: VIEWPORT_GUTTER_PX,
          left: VIEWPORT_GUTTER_PX,
          bottom: VIEWPORT_GUTTER_PX + bottomTabBarHeight(),
        },
        flipAlignment: false,
      })),
      shift({ padding: VIEWPORT_GUTTER_PX }),
    ],
    whileElementsMounted: autoUpdate,
  })

  useLayoutEffect(() => {
    setReference(anchorRef.current)
  }, [setReference, anchorRef])

  const dismiss = useCallback(() => {
    const focusWasInside =
      hintRef.current?.contains(document.activeElement) ?? false
    dismissOnAccount()
    onClose()
    // The focused control is about to unmount; without this the keyboard
    // user's place in the page drops to <body>.
    if (focusWasInside) {
      anchorRef.current?.querySelector<HTMLElement>('button, a')?.focus()
    }
  }, [anchorRef, dismissOnAccount, onClose, hintRef])

  // Bubble phase on purpose: a Radix menu or popover handles its Escape in the
  // capture phase and marks it defaultPrevented, so an Escape that closed one
  // of those is not read as dismissing this hint.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented) dismiss()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [dismiss])

  return (
    <div
      ref={setFloating}
      role="status"
      data-testid="first-save-hint"
      data-placement={placement}
      style={{
        ...floatingStyles,
        // Hidden until placed, so it never flashes at the viewport origin.
        visibility: isPositioned ? undefined : 'hidden',
      }}
      className="z-50 flex w-[300px] max-w-[calc(100vw-2rem)] items-start gap-3 rounded-md border border-border bg-popover px-3 py-2.5 text-left font-sans text-sm normal-case leading-snug tracking-normal whitespace-normal text-popover-foreground shadow-md"
    >
      <p className="flex-1">
        Saved. Find it on your{' '}
        <Link
          href="/"
          onClick={dismiss}
          className="text-primary underline underline-offset-2"
        >
          home page
        </Link>{' '}
        and in{' '}
        <Link
          href="/library"
          onClick={dismiss}
          className="text-primary underline underline-offset-2"
        >
          Library
        </Link>
        .
      </p>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        className="-mr-1 mt-0.5 shrink-0 rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <X className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </div>
  )
}
