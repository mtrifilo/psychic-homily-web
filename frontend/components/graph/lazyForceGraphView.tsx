'use client'

/**
 * createLazyForceGraphView (PSY-1359) — the shared `dynamic(ssr:false)` wrapper
 * that splits ForceGraphView (and react-force-graph-2d underneath) into an async
 * chunk fetched only when a below-the-fold graph section mounts, keeping anything
 * graph-shaped out of the host page's initial JS (PSY-868).
 *
 * Shared by InlineGraph (PSY-837) and HomeSceneGraph (PSY-1344); they differ only
 * in the height-reserving `loading` skeleton (each surface reserves CLS height its
 * own way), so that is the one parameter. Peer ForceGraphView consumers (Scene /
 * Venue / Collection graphs) keep a STATIC import on purpose — there the graph is
 * the page's primary content, so a split would only add a chunk round-trip.
 *
 * ssr:false costs nothing (the canvas never renders server-side). NOTE: in the
 * App Router a failed chunk fetch THROWS to the nearest error boundary (it does
 * NOT re-invoke `loading` with an error), so callers MUST wrap the mount in
 * GraphSectionErrorBoundary — `loading` here is only the happy-path skeleton.
 *
 * Called at MODULE scope by each surface (not in render): `dynamic()` must not be
 * re-created per render, and ssr:false requires a client module (this file is one).
 */

import dynamic from 'next/dynamic'
import type { ComponentType, ReactNode } from 'react'
import type { ForceGraphViewProps } from './ForceGraphView'

export function createLazyForceGraphView(
  loading: ReactNode,
): ComponentType<ForceGraphViewProps> {
  return dynamic(
    () => import('./ForceGraphView').then(m => ({ default: m.ForceGraphView })),
    {
      ssr: false,
      loading: () => <>{loading}</>,
    },
  )
}
