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
} from './types'

// Hooks
export {
  useScenes,
  useSceneDetail,
  useSceneArtists,
  useSceneGenres,
} from './hooks'

// Components
export { SceneList, ScenePulse as ScenePulseCard, SceneDetailView } from './components'
