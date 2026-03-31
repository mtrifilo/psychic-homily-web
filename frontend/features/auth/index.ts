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
  APIToken,
  ActivityDay,
  ActivityHeatmapResponse,
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

// Hooks — Favorite Venues
export {
  useFavoriteVenues,
  useIsVenueFavorited,
  useFavoriteVenue,
  useUnfavoriteVenue,
  useFavoriteVenueToggle,
  useFavoriteVenueShows,
} from './hooks'

// Hooks — Contributor Profile
export {
  usePublicProfile,
  usePublicContributions,
  useActivityHeatmap,
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
