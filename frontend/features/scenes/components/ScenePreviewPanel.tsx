'use client'

import { useEffect, useRef } from 'react'
import { ScenePreviewContent } from './ScenePreviewContent'
import type { SceneListItem } from '../types'

interface ScenePreviewPanelProps {
  scene: SceneListItem
  onClose: () => void
  /**
   * Where focus goes when the panel closes (PSY-1313) — AtlasGlobe passes the
   * "Search scenes" trigger, the page's keyboard entry point. An EXPLICIT ref
   * on purpose: capturing document.activeElement at mount was tried and fails
   * live — on the search path cmdk re-focuses its own input after any
   * synchronous hand-off, so the capture lands on an element the popover's
   * exit animation is about to remove.
   */
  returnFocusTo?: React.RefObject<HTMLElement | null>
}

/**
 * The radio.garden-style payoff: clicking a globe dot opens this in-place summary
 * of the city's scene (counts + a few active artists) with a link INTO the full
 * scene page — so the user gets immediate context without leaving the globe.
 * The body (embed + this-week + roster) is ScenePreviewContent, shared with the
 * mobile scene list (PSY-1311); this panel owns the desktop chrome (aside,
 * header, close, Esc).
 */
export function ScenePreviewPanel({
  scene,
  onClose,
  returnFocusTo,
}: ScenePreviewPanelProps) {
  const closeRef = useRef<HTMLButtonElement>(null)
  const asideRef = useRef<HTMLElement>(null)

  // Keyboard a11y for the non-modal panel: focus the close control on open and
  // dismiss on Escape (every other dismissable surface in the app supports Esc).
  // Deliberately NOT the Radix Sheet — that's modal and would block the globe;
  // this panel stays non-modal so the globe is still interactive behind it.
  //
  // PSY-1313: focus the close control on open; hand focus to returnFocusTo on
  // close. Mount-only on purpose: switching scenes keeps the panel mounted and
  // must not re-run either move.
  useEffect(() => {
    // Both nodes exist at mount and are stable for the panel's lifetime —
    // capture them here so the cleanup doesn't read refs post-unmount.
    const aside = asideRef.current
    const returnTarget = returnFocusTo?.current ?? null
    closeRef.current?.focus()
    return () => {
      // Restore only when focus is still OURS to hand back: inside the closing
      // panel, or already dropped to <body>. The panel is non-modal (no focus
      // trap) and Esc is document-level, so the user may have tabbed elsewhere
      // — yanking focus back from a header link they're on would be worse than
      // no restore (the same containment rule Radix FocusScope applies).
      const active = document.activeElement
      const focusIsOurs =
        active === document.body ||
        (active instanceof HTMLElement && aside !== null && aside.contains(active))
      if (focusIsOurs && returnTarget?.isConnected) returnTarget.focus()
    }
    // returnFocusTo is a stable ref container from AtlasGlobe; this effect is
    // mount-only by design (see the comment above).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      // Esc while typing in a field (e.g. the reopened scene-search input)
      // belongs to that surface, not the panel — same guard idiom as the "/"
      // shortcut in AtlasSearch. Without it one Escape closes BOTH layers.
      const target = e.target as HTMLElement | null
      if (
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable)
      ) {
        return
      }
      onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <aside
      ref={asideRef}
      className="absolute right-0 top-0 z-10 flex h-full w-full max-w-sm flex-col gap-4 overflow-y-auto border-l border-border bg-background/95 p-5 backdrop-blur"
      aria-label={`${scene.city}, ${scene.state} scene`}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-lg font-semibold leading-tight">
            {scene.city}, {scene.state}
          </h2>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {scene.upcoming_show_count} upcoming · {scene.venue_count} venues
          </p>
        </div>
        <button
          ref={closeRef}
          type="button"
          onClick={onClose}
          aria-label="Close scene preview"
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <span aria-hidden>×</span>
        </button>
      </div>

      {/* flex-1 grows the body so its mt-auto scene link pins to the panel
          bottom; the shared body itself is layout-neutral (PSY-1311). */}
      <ScenePreviewContent scene={scene} className="flex-1" />
    </aside>
  )
}
