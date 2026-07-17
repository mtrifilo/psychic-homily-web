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
 * Known limits, accepted for a best-effort escape hatch:
 * - The graph payload carries the artist's raw home city, not its CBSA metro,
 *   so a suburb (Tempe) doesn't exact-match its metro scene (Phoenix) and
 *   degrades to the same-state-liveliest fallback.
 * - A home match requires BOTH city and state: city names alias across states
 *   (Portland OR/ME), so a missing state makes a "home" claim a guess — the
 *   activity fallback is safer than a confidently wrong link.
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
      stateKey !== '' &&
      normalize(scene.city) === cityKey &&
      normalize(scene.state) === stateKey
    if (isHome) home.push(scene)
    else if (stateKey !== '' && normalize(scene.state) === stateKey) sameState.push(scene)
    else elsewhere.push(scene)
  }
  home.sort(compareScenesByActivity)
  sameState.sort(compareScenesByActivity)
  elsewhere.sort(compareScenesByActivity)

  return [...home, ...sameState, ...elsewhere].slice(0, MAX_SCENE_ESCAPE_HATCHES)
}
