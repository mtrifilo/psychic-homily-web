/**
 * The Atlas camera carried across map teardowns.
 *
 * Module scope (not a ref) on purpose: it survives not only Cache Components'
 * hide but also REAL unmounts of the canvas, such as the <640px mobile-gate
 * flip, which unmounts it entirely. Single-instance surface (one Atlas globe
 * per app), so shared module state is safe. Deliberately a DATA cache, not an
 * init guard: the map is still created fresh on every show, and the one
 * pattern PSY-1284 proved fatal was a guard ref that survives hide and skips
 * re-init. Without this, nav-away/back would reset the camera to the initial
 * POV, because the map instance is new each show.
 *
 * Its own module so the entry points that must OVERRIDE a saved camera (a
 * URL that names where to open) can clear it without importing the canvas
 * (and maplibre with it).
 */

export interface AtlasCamera {
  center: [number, number]
  zoom: number
}

let savedCamera: AtlasCamera | null = null

/** The camera to reopen on, or null to use the caller's initial focus. */
export function readAtlasCamera(): AtlasCamera | null {
  return savedCamera
}

export function saveAtlasCamera(camera: AtlasCamera): void {
  savedCamera = camera
}

/**
 * Drops the carried camera, so the next map opens on its initial focus.
 *
 * The entry a URL names outranks where the session last left the camera: a
 * link that says which city to open on has to move the map, or it does
 * nothing at all for a visitor who already used the Atlas this session.
 */
export function clearAtlasCamera(): void {
  savedCamera = null
}
