'use client'

import { useEffect, useRef } from 'react'
import { ScenePreviewContent } from './ScenePreviewContent'
import type { SceneListItem } from '../types'

interface ScenePreviewPanelProps {
  scene: SceneListItem
  onClose: () => void
}

/**
 * The radio.garden-style payoff: clicking a globe dot opens this in-place summary
 * of the city's scene (counts + a few active artists) with a link INTO the full
 * scene page — so the user gets immediate context without leaving the globe.
 * The body (embed + this-week + roster) is ScenePreviewContent, shared with the
 * mobile scene list (PSY-1311); this panel owns the desktop chrome (aside,
 * header, close, Esc).
 */
export function ScenePreviewPanel({ scene, onClose }: ScenePreviewPanelProps) {
  const closeRef = useRef<HTMLButtonElement>(null)

  // Keyboard a11y for the non-modal panel: focus the close control on open and
  // dismiss on Escape (every other dismissable surface in the app supports Esc).
  // Deliberately NOT the Radix Sheet — that's modal and would block the globe;
  // this panel stays non-modal so the globe is still interactive behind it.
  useEffect(() => {
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <aside
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
