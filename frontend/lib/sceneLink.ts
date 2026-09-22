import type { components } from '@/types/api'

/**
 * The scene page a detail payload links to. The backend sends it only when
 * that page serves (a US place whose scene clears the venue floor), so its
 * presence is the whole test: never build a scene href from a city and state
 * on the client.
 */
export type SceneLink = components['schemas']['SceneLinkResponse']

export function sceneLinkHref(scene: SceneLink): string {
  return `/scenes/${encodeURIComponent(scene.slug)}`
}
