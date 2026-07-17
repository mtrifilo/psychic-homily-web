/**
 * Scene escape hatches for the Observatory's "No mapped connections yet"
 * empty state (PSY-1474 F4, Gazelle search-miss pattern: never dead-end —
 * hand the user clickable ways onward).
 *
 * The board copy asks for "2 scene links from the artist's metro". Scenes are
 * metro-keyed (PSY-1255), so the artist's own metro contributes at most one
 * scene; the second slot (and both slots for artists outside any scene-
 * threshold city) falls back geographically outward: same-state scenes first,
 * then the liveliest scenes overall — ordered by the shared
 * `compareScenesByActivity` rule so this surface can't disagree with Atlas.
 *
 * Pure selection logic, no React — easy to unit-test.
 */

import { compareScenesByActivity } from '@/features/scenes/components/globeScale'
import type { SceneListItem } from '@/features/scenes/types'

export const MAX_SCENE_ESCAPE_HATCHES = 2

function normalize(value: string | null | undefined): string {
  return (value ?? '').trim().toLowerCase()
}

export function pickSceneEscapeHatches(
  scenes: SceneListItem[],
  city: string | null | undefined,
  state: string | null | undefined,
): SceneListItem[] {
  const cityKey = normalize(city)
  const stateKey = normalize(state)

  const home: SceneListItem[] = []
  const sameState: SceneListItem[] = []
  const elsewhere: SceneListItem[] = []
  for (const scene of scenes) {
    const isHome =
      cityKey !== '' &&
      normalize(scene.city) === cityKey &&
      (stateKey === '' || normalize(scene.state) === stateKey)
    if (isHome) home.push(scene)
    else if (stateKey !== '' && normalize(scene.state) === stateKey) sameState.push(scene)
    else elsewhere.push(scene)
  }
  sameState.sort(compareScenesByActivity)
  elsewhere.sort(compareScenesByActivity)

  return [...home, ...sameState, ...elsewhere].slice(0, MAX_SCENE_ESCAPE_HATCHES)
}
