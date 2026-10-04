'use client'

import { useEffect, useRef } from 'react'
import { BottomSheet } from '@/components/ui/bottom-sheet'
import { ScenePreviewContent } from './ScenePreviewContent'
import { ATLAS_SHEET_TOP_INSET_PX } from '../cityView'
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
  /**
   * `panel` docks to the map's right edge; `sheet` is a bottom sheet opening
   * at Half, for panes too narrow for a side panel.
   */
  presentation?: 'panel' | 'sheet'
}

/**
 * The radio.garden-style payoff: clicking a globe dot opens this in-place summary
 * of the city's scene (counts + a few active artists) with a link INTO the full
 * scene page — so the user gets immediate context without leaving the globe.
 * The body (embed + this-week + roster) is ScenePreviewContent, shared with the
 * mobile scene list (PSY-1311); this component owns the chrome around it: a
 * right-docked aside, or a bottom sheet opening at Half on narrow panes.
 */
export function ScenePreviewPanel({
  scene,
  onClose,
  returnFocusTo,
  presentation = 'panel',
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
  // must not re-run either move. The caller keys the panel on its
  // presentation, so a layout switch is a remount, never a mid-life swap.
  useEffect(() => {
    // Both nodes exist at mount and are stable for the panel's lifetime —
    // capture them here so the cleanup doesn't read refs post-unmount.
    const aside = asideRef.current
    const returnTarget = returnFocusTo?.current ?? null
    closeRef.current?.focus()
    return () => {
      // Restore only when focus is still OURS to hand back: inside the closing
      // panel, or already dropped to <body>. The panel is non-modal (no focus
      // trap) and Esc is not scoped to it, so the user may have tabbed elsewhere
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

  // The sheet dismisses on Escape through the shared layer stack instead.
  const isSheet = presentation === 'sheet'
  useEffect(() => {
    if (isSheet) return
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
  }, [onClose, isSheet])

  const title = `${scene.city}, ${scene.state}`
  const stats = `${scene.upcoming_show_count} upcoming · ${scene.venue_count} venues`

  if (isSheet) {
    return (
      <BottomSheet
        ref={asideRef}
        closeRef={closeRef}
        title={title}
        label={`${title} scene`}
        aria-label={`${title} scene`}
        data-testid="atlas-scene-preview-sheet"
        defaultDetent="half"
        onClose={onClose}
        closeLabel="Close scene preview"
        topInsetPx={ATLAS_SHEET_TOP_INSET_PX}
      >
        <div className="flex min-h-full flex-col gap-4">
          <p className="font-mono text-sm text-muted-foreground">{stats}</p>
          <ScenePreviewContent scene={scene} className="flex-1" />
        </div>
      </BottomSheet>
    )
  }

  return (
    <aside
      ref={asideRef}
      className="absolute right-0 top-0 z-10 flex h-full w-full max-w-sm flex-col gap-4 overflow-y-auto border-l border-border bg-background/95 p-5 backdrop-blur"
      aria-label={`${scene.city}, ${scene.state} scene`}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-lg font-semibold leading-tight">{title}</h2>
          <p className="mt-1 font-mono text-sm text-muted-foreground">{stats}</p>
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
