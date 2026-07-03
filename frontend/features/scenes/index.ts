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
} from './hooks'

// Components
export { SceneList, ScenePulse as ScenePulseCard, SceneDetailView } from './components'

// Cross-surface rules (PSY-1344): the ONE liveliest-first ordering (globe
// labels / search / mobile list / homepage graph default) and the ONE
// "how many artists" phrase (visual header + canvas aria-label).
export { compareScenesByActivity } from './components/globeScale'
export { sceneArtistCountPhrase } from './components/sceneGraphCopy'
