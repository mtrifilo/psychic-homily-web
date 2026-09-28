'use client'

import { useCallback, type ReactElement } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { useDismissFirstSaveHint } from '@/features/shows/hooks/useFirstSaveHint'

// Minimum distance between the hint and either edge of the viewport. The
// hint's max width below (100vw - 2rem) is this gutter on both sides.
const VIEWPORT_GUTTER_PX = 16

interface FirstSaveHintProps {
  /** Whether the hint is showing. */
  open: boolean
  /** Which edge of the Save control the hint lines up with. */
  align: 'start' | 'end'
  /** Close the hint locally. Dismissal on the account happens here. */
  onClose: () => void
  /** The Save control's own wrapper, which the hint anchors to. Always
   *  rendered through this component, open or not, so opening the hint never
   *  remounts the control (and never drops its focus). */
  children: ReactElement
}

/**
 * The one-time note that follows a viewer's first saved show, naming where the
 * save went. It is portalled and anchored under the Save control, so it
 * overlays what follows instead of pushing a dense row apart, and no clipping
 * ancestor (a scrolling table, an `overflow-hidden` module) can cut it off.
 */
export function FirstSaveHint({
  open,
  align,
  onClose,
  children,
}: FirstSaveHintProps) {
  return (
    <Popover open={open}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      {open ? <FirstSaveHintContent align={align} onClose={onClose} /> : null}
    </Popover>
  )
}

/**
 * Mounted only while open, so the account write it owns (and the query
 * client that write needs) is never touched by the closed Save controls on a
 * list page. Every way of closing it (the dismiss control, either link,
 * Escape) stamps the account, so it never opens again on any device.
 */
function FirstSaveHintContent({
  align,
  onClose,
}: Pick<FirstSaveHintProps, 'align' | 'onClose'>) {
  const { mutate: dismissOnAccount } = useDismissFirstSaveHint()
  const dismiss = useCallback(() => {
    dismissOnAccount()
    onClose()
  }, [dismissOnAccount, onClose])

  return (
    <PopoverContent
      side="bottom"
      align={align}
      sideOffset={8}
      collisionPadding={VIEWPORT_GUTTER_PX}
      // A hint, not a dialog: focus stays on the Save control, and a click
      // elsewhere on the page leaves it open until the viewer closes it.
      onOpenAutoFocus={event => event.preventDefault()}
      onCloseAutoFocus={event => event.preventDefault()}
      onInteractOutside={event => event.preventDefault()}
      // Radix calls this only while the hint is the topmost layer, so an
      // Escape that closes a menu opened over it is not a dismissal; nor is
      // one another handler already consumed.
      onEscapeKeyDown={event => {
        if (!event.defaultPrevented) dismiss()
      }}
      role="status"
      data-testid="first-save-hint"
      className="flex w-[300px] max-w-[calc(100vw-2rem)] items-start gap-3 border-border px-3 py-2.5 text-left font-sans text-sm normal-case leading-snug tracking-normal whitespace-normal shadow-md"
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
    </PopoverContent>
  )
}
