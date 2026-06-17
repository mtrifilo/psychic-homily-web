// Public API for the profile feature module
//
// Profile-surface rendering and editing components. Profile data (types +
// hooks) lives in `features/auth` and `features/contributions`; this module
// owns only the presentation/editing surfaces consumed by the profile routes.

// Components
export {
  ActivityHeatmap,
  UserTierBadge,
  ContributionStatsGrid,
  ContributionTimeline,
  ProfileSections,
  ProfileSectionsEditor,
  PrivacySettingsPanel,
  PercentileRankings,
  PublicProfile,
  ContributorProfilePreview,
  TierAdvancementCard,
  GetStartedChecklist,
  ProfileStatsSidebar,
} from './components'
