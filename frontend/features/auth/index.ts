// Public API for the auth feature module

// Types
export type {
  ProfileVisibility,
  PrivacyLevel,
  UserTier,
  PrivacySettings,
  ContributionStats,
  PublicProfileResponse,
  ContributionEntry,
  ContributionsResponse,
  ProfileSectionResponse,
  ProfileSectionsResponse,
  CreateSectionInput,
  UpdateSectionInput,
  UpdateVisibilityInput,
  UpdatePrivacyInput,
  PercentileRanking,
  PercentileRankings,
  APIToken,
  ActivityDay,
  ActivityHeatmapResponse,
  FollowingEntity,
  UserFollowingResponse,
  AuthoredFieldNote,
  UserFieldNotesResponse,
} from './types'

// Hooks — Auth
export {
  useLogin,
  useRegister,
  useLogout,
  useProfile,
  useUpdateProfile,
  useRefreshToken,
  useIsAuthenticated,
  useSendVerificationEmail,
  useConfirmVerification,
  useChangePassword,
  useSendMagicLink,
  useVerifyMagicLink,
  useDeletionSummary,
  useDeleteAccount,
  useExportData,
  useOAuthAccounts,
  useUnlinkOAuthAccount,
  useRecoverAccount,
  useRequestAccountRecovery,
  useConfirmAccountRecovery,
  useGenerateCLIToken,
  useAPITokens,
  useCreateAPIToken,
  useRevokeAPIToken,
} from './hooks'

// Hooks — Calendar Feed
export {
  useCalendarTokenStatus,
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from './hooks'

// Hooks — Favorite Cities
export { useSetFavoriteCities } from './hooks'

// Hooks — Chart defaults (PSY-1423)
export { useSetChartDefaults } from './hooks'
export type { ChartDefaults } from './hooks'

// Hooks — Tier-change / edit-review email preferences (PSY-756 / PSY-807)
export { useSetTierEditNotificationPreference } from './hooks'
export type { TierEditNotificationUpdate } from './hooks'

// Hooks — Contributor Profile
export {
  usePublicProfile,
  usePublicContributions,
  useActivityHeatmap,
  usePercentileRankings,
  useUserFollowing,
  useUserFieldNotes,
  useOwnContributorProfile,
  useOwnContributions,
  useOwnSections,
  useUpdateVisibility,
  useUpdatePrivacy,
  useCreateSection,
  useUpdateSection,
  useDeleteSection,
} from './hooks'

// Components
export { LoginPromptDialog, PasskeyRegisterButton } from './components'

// Components — Account settings (sole consumer: app/profile)
export { SettingsPanel } from './components/settings'
