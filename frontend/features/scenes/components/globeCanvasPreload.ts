/**
 * Starts loading the GlobeCanvas module (and with it MapLibre, its CSS and,
 * on evaluation, the worker pool) without rendering it. It resolves the same
 * module as AtlasGlobe's next/dynamic import, so calling it early only moves
 * the download earlier; idempotent.
 *
 * A failure is swallowed here: the dynamic() render path loads the module
 * again and owns the error state and its retry.
 */
export function preloadGlobeCanvas(): void {
  import('./GlobeCanvas').catch(() => {})
}
