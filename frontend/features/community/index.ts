// Public API for the community feature module.
// Other features should import from '@/features/community', not from internal paths.

// Types
export type {
  LeaderboardDimension,
  LeaderboardPeriod,
  LeaderboardEntry,
  LeaderboardResponse,
} from './types'
export { DIMENSION_LABELS, PERIOD_LABELS } from './types'

// Hooks
export { useLeaderboard } from './hooks'

// Components
export { LeaderboardPage } from './components'
