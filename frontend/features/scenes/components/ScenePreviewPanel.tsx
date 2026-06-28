'use client'

import { useEffect, useRef } from 'react'
import Link from 'next/link'
import { useSceneArtists } from '../hooks'
import type { SceneListItem } from '../types'

interface ScenePreviewPanelProps {
  scene: SceneListItem
  onClose: () => void
}

/**
 * The radio.garden-style payoff: clicking a globe dot opens this in-place summary
 * of the city's scene (counts + a few active artists) with a link INTO the full
 * scene page — so the user gets immediate context without leaving the globe.
 */
export function ScenePreviewPanel({ scene, onClose }: ScenePreviewPanelProps) {
  const { data, isLoading } = useSceneArtists({ slug: scene.slug, limit: 6 })
  const artists = data?.artists ?? []
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

      <div>
        <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Local artists
        </h3>
        {isLoading ? (
          <p className="mt-2 text-sm text-muted-foreground">Loading…</p>
        ) : artists.length > 0 ? (
          <ul className="mt-2 flex flex-col gap-1">
            {artists.map((a) => (
              <li key={a.id} className="flex items-center gap-1.5">
                {/* Reserve the dot's width on every row so names stay aligned
                    whether or not the band is active. */}
                <span className="flex h-1.5 w-1.5 shrink-0" aria-hidden>
                  {a.is_active && (
                    <span className="h-1.5 w-1.5 rounded-full bg-success-foreground" />
                  )}
                </span>
                <Link
                  href={`/artists/${a.slug}`}
                  className="text-sm underline-offset-4 hover:underline"
                >
                  {a.name}
                </Link>
                {a.is_active && <span className="sr-only">(active)</span>}
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-2 text-sm text-muted-foreground">
            No artists based here yet.
          </p>
        )}
      </div>

      <Link
        href={`/scenes/${scene.slug}`}
        className="mt-auto inline-flex items-center gap-1 text-sm font-medium text-primary underline-offset-4 hover:underline"
      >
        Open scene →
      </Link>
    </aside>
  )
}
