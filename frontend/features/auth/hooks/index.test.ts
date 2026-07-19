import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Grouped by source
// module so a missing module surfaces an obvious cluster of failures.
describe('auth hooks barrel', () => {
  it('re-exports the core auth hooks', () => {
    expect(typeof hooks.useLogin).toBe('function')
    expect(typeof hooks.useRegister).toBe('function')
    expect(typeof hooks.useLogout).toBe('function')
    expect(typeof hooks.useProfile).toBe('function')
    expect(typeof hooks.useUpdateProfile).toBe('function')
    expect(typeof hooks.useRefreshToken).toBe('function')
    expect(typeof hooks.useIsAuthenticated).toBe('function')
    expect(typeof hooks.useSendVerificationEmail).toBe('function')
    expect(typeof hooks.useConfirmVerification).toBe('function')
    expect(typeof hooks.useChangePassword).toBe('function')
    expect(typeof hooks.useSendMagicLink).toBe('function')
    expect(typeof hooks.useVerifyMagicLink).toBe('function')
    expect(typeof hooks.useDeletionSummary).toBe('function')
    expect(typeof hooks.useDeleteAccount).toBe('function')
    expect(typeof hooks.useExportData).toBe('function')
    expect(typeof hooks.useOAuthAccounts).toBe('function')
    expect(typeof hooks.useUnlinkOAuthAccount).toBe('function')
    expect(typeof hooks.useRecoverAccount).toBe('function')
    expect(typeof hooks.useRequestAccountRecovery).toBe('function')
    expect(typeof hooks.useConfirmAccountRecovery).toBe('function')
    expect(typeof hooks.useGenerateCLIToken).toBe('function')
    expect(typeof hooks.useAPITokens).toBe('function')
    expect(typeof hooks.useCreateAPIToken).toBe('function')
    expect(typeof hooks.useRevokeAPIToken).toBe('function')
  })

  it('re-exports the calendar-feed hooks', () => {
    expect(typeof hooks.useCalendarTokenStatus).toBe('function')
    expect(typeof hooks.useCreateCalendarToken).toBe('function')
    expect(typeof hooks.useDeleteCalendarToken).toBe('function')
  })

  it('re-exports the favorite-cities hook', () => {
    expect(typeof hooks.useSetFavoriteCities).toBe('function')
  })

  it('re-exports the chart-defaults hook', () => {
    expect(typeof hooks.useSetChartDefaults).toBe('function')
  })

  it('re-exports the contributor-profile hooks', () => {
    expect(typeof hooks.usePublicProfile).toBe('function')
    expect(typeof hooks.usePublicContributions).toBe('function')
    expect(typeof hooks.useActivityHeatmap).toBe('function')
    expect(typeof hooks.usePercentileRankings).toBe('function')
    expect(typeof hooks.useOwnContributorProfile).toBe('function')
    expect(typeof hooks.useOwnContributions).toBe('function')
    expect(typeof hooks.useOwnSections).toBe('function')
    expect(typeof hooks.useAdvancementProgress).toBe('function')
    expect(typeof hooks.useUpdateVisibility).toBe('function')
    expect(typeof hooks.useUpdatePrivacy).toBe('function')
    expect(typeof hooks.useCreateSection).toBe('function')
    expect(typeof hooks.useUpdateSection).toBe('function')
    expect(typeof hooks.useDeleteSection).toBe('function')
  })
})
