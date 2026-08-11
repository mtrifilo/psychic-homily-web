// Public API for the scenes feature module.
// Other features should import from '@/features/scenes', not from internal paths.

// Types
export type {
  SceneListItem,
  SceneListResponse,
  SceneStats,
  ScenePulse,
  SceneDetail,
  SceneArtist,
  SceneArtistsResponse,
  GenreCount,
  SceneGenreResponse,
  SceneGraphInfo,
  SceneGraphCluster,
  SceneGraphNode,
  SceneGraphLink,
  SceneGraphResponse,
} from './types'

// Hooks
export {
  useScenes,
  useSceneDetail,
  useSceneArtists,
  useSceneGenres,
  useSceneGraph,
  useSetSceneDigestPreference,
} from './hooks'

// Components
// NOTE: SceneDetailView is intentionally omitted (PSY-1772). The route page
// imports it directly via `dynamic()` from
// '@/features/scenes/components/SceneDetail' so Turbopack evicts it from the
// global shared client chunk (loaded on every route). Re-adding it here
// re-hoists SceneDetail.tsx into that chunk.
export { SceneList, ScenePulse as ScenePulseCard } from './components'

// Cross-surface rules (PSY-1344): the ONE liveliest-first ordering (globe
// labels / search / mobile list / homepage graph default) and the ONE
// "how many artists" phrase (visual header + canvas aria-label).
export { compareScenesByActivity } from './components/globeScale'
export { sceneArtistCountPhrase } from './components/sceneGraphCopy'
