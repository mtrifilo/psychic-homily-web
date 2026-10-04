import { prefetchGlobeLand } from '../basemap/globeSurface'
import { isAtlasCompactViewport } from '../atlasViewport'

/**
 * AtlasGlobe renders the map only in a container at least this wide, and a
 * scene list below it.
 */
export const GLOBE_BREAKPOINT_PX = 640

/**
 * Starts what the Atlas map needs before AtlasGlobe renders it: the
 * GlobeCanvas module (MapLibre, its CSS and, on evaluation, the worker pool)
 * and, on a compact viewport (the light globe), the land data. The module import resolves the
 * same module as AtlasGlobe's next/dynamic import, so calling this early only
 * moves the downloads earlier. Idempotent.
 *
 * A failed module fetch is swallowed here: the dynamic() render path loads
 * the module again and owns the error state and its retry.
 */
export function preloadAtlasMap(): void {
  import('./GlobeCanvas').catch(() => {})
  if (isAtlasCompactViewport()) prefetchGlobeLand()
}

/**
 * {@link preloadAtlasMap} at page load, before hydration finishes, when the
 * viewport is wide enough that the map will render. The viewport stands in
 * for AtlasGlobe's container, which is not measured yet: where they differ
 * (a side navigation narrowing the container) this either starts the
 * downloads for a scene list or leaves them to AtlasGlobe's own preload once
 * the container is measured.
 */
export function preloadAtlasMapAtPageLoad(): void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
  if (window.matchMedia(`(min-width: ${GLOBE_BREAKPOINT_PX}px)`).matches) preloadAtlasMap()
}
