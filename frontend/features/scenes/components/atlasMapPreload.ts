import { prefetchGlobeLand } from '../basemap/globeSurface'

/**
 * Starts what the Atlas map needs before AtlasGlobe renders it: the
 * GlobeCanvas module (MapLibre, its CSS and, on evaluation, the worker pool)
 * and, for the light globe, the land data. The module import resolves the
 * same module as AtlasGlobe's next/dynamic import, so calling this early only
 * moves the downloads earlier. Idempotent.
 *
 * A failed module fetch is swallowed here: the dynamic() render path loads
 * the module again and owns the error state and its retry.
 */
export function preloadAtlasMap({ lightGlobe }: { lightGlobe: boolean }): void {
  import('./GlobeCanvas').catch(() => {})
  if (lightGlobe) prefetchGlobeLand()
}
