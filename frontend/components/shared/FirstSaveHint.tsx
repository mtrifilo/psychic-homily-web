'use client'

import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import Link from 'next/link'
import { X } from 'lucide-react'
import { useDismissFirstSaveHint } from '@/features/shows/hooks/useFirstSaveHint'
import { cn } from '@/lib/utils'

// Minimum distance between the hint and either edge of the viewport.
const VIEWPORT_GUTTER_PX = 16

interface FirstSaveHintProps {
  /** Which edge of the Save control the hint lines up with. */
  align: 'start' | 'end'
  /** Close the hint locally. Dismissal on the account happens here. */
  onClose: () => void
}

/**
 * The one-time note that follows a viewer's first saved show, naming where the
 * save went. Anchored under the Save control and absolutely positioned, so it
 * overlays what follows instead of pushing a dense row apart. Every way of
 * closing it (the dismiss control, either link, Escape) stamps the account,
 * so it never opens again on any device.
 */
export function FirstSaveHint({ align, onClose }: FirstSaveHintProps) {
  const { mutate: dismissOnAccount } = useDismissFirstSaveHint()
  const dismiss = useCallback(() => {
    dismissOnAccount()
    onClose()
  }, [dismissOnAccount, onClose])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') dismiss()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [dismiss])

  // Anchored to the control, then nudged back inside the viewport when that
  // anchor would push it past either edge (a Save control mid-row on a phone).
  const hintRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const hint = hintRef.current
    if (!hint) return
    const keepInViewport = () => {
      const viewportWidth = document.documentElement.clientWidth
      if (!viewportWidth) return
      hint.style.transform = ''
      const { left, right } = hint.getBoundingClientRect()
      let shift = 0
      if (right > viewportWidth - VIEWPORT_GUTTER_PX) {
        shift = viewportWidth - VIEWPORT_GUTTER_PX - right
      }
      if (left + shift < VIEWPORT_GUTTER_PX) {
        shift = VIEWPORT_GUTTER_PX - left
      }
      hint.style.transform = shift ? `translateX(${shift}px)` : ''
    }
    keepInViewport()
    window.addEventListener('resize', keepInViewport)
    return () => window.removeEventListener('resize', keepInViewport)
  }, [])

  return (
    <div
      ref={hintRef}
      role="status"
      data-testid="first-save-hint"
      className={cn(
        'absolute top-full z-50 mt-2 flex w-[300px] max-w-[calc(100vw-2rem)] items-start gap-3 rounded-md border border-border bg-popover px-3 py-2.5 text-left font-sans text-sm normal-case leading-snug tracking-normal whitespace-normal text-popover-foreground shadow-md',
        align === 'start' ? 'left-0' : 'right-0'
      )}
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
