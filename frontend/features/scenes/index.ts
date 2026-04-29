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
