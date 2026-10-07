import { prefetchGlobeLand } from '../basemap/globeSurface'
import { isAtlasCompactViewport } from '../atlasViewport'
import { loadGlobeCanvas } from './loadGlobeCanvas'

/**
 * Starts what the Atlas map needs before AtlasGlobe renders it: the
 * GlobeCanvas module (MapLibre and its CSS) and, on a compact viewport (the
 * light globe), the land data. It loads the canvas through loadGlobeCanvas,
 * the loader AtlasGlobe's next/dynamic calls, so calling this early only
 * moves the downloads earlier. Idempotent.
 *
 * A failed module fetch is swallowed here: the dynamic() render path imports
 * the module again, and a failure there reaches the error boundary around
 * the canvas.
 */
export function preloadAtlasMap(): void {
  loadGlobeCanvas().catch(() => {})
  if (isAtlasCompactViewport()) prefetchGlobeLand()
}
