'use client'

import { useCallback, useEffect, useLayoutEffect, type RefObject } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'
import { autoUpdate, offset, shift, useFloating } from '@floating-ui/react-dom'
import { useDismissFirstSaveHint } from '@/features/shows/hooks/useFirstSaveHint'

// Minimum distance between the hint and either edge of the viewport. The
// hint's max width below (100vw - 2rem) is this gutter on both sides.
const VIEWPORT_GUTTER_PX = 16

interface FirstSaveHintProps {
  /** The Save control's wrapper. The hint positions against it, and focus
   *  returns to the first control inside it when the hint closes from the
   *  keyboard. */
  anchorRef: RefObject<HTMLElement | null>
  /** Which edge of the Save control the hint lines up with. */
  align: 'start' | 'end'
  /** Close the hint locally. Dismissal on the account happens here. */
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
 * outside clicks. Every way of closing it (the dismiss control, either link,
 * Escape) stamps the account, so it never opens again on any device.
 */
export function FirstSaveHint({ anchorRef, align, onClose }: FirstSaveHintProps) {
  const { mutate: dismissOnAccount } = useDismissFirstSaveHint()
  const {
    floatingStyles,
    isPositioned,
    refs: { setReference, setFloating, floating: hintRef },
  } = useFloating({
    strategy: 'fixed',
    placement: align === 'start' ? 'bottom-start' : 'bottom-end',
    middleware: [offset(8), shift({ padding: VIEWPORT_GUTTER_PX })],
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
      data-align={align}
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
