import type { components } from '@/types/api'

/**
 * The scene page a detail payload links to. The backend sends it only when
 * that page serves (a US place whose scene clears the venue floor), so its
 * presence is the whole test: never build a scene href from a city and state
 * on the client. Build the href with `entityHref('/scenes', scene.slug)`.
 */
export type SceneLink = components['schemas']['SceneLinkResponse']
