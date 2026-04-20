import type { UserTier } from '@/features/auth'

export interface TierRequirement {
  text: string
}

export interface TierInfo {
  tier: UserTier
  label: string
  summary: string
  permissions: string[]
  // Requirements to ADVANCE to this tier (empty for new_user).
  // Sourced from backend/internal/services/admin/auto_promotion.go constants.
  advancementFrom?: UserTier
  advancementRequirements?: TierRequirement[]
}

// Mirrors promotion thresholds in auto_promotion.go so the UI never drifts
// from the promotion rules that actually run server-side. When those
// constants change, update this file too.
export const TIERS: TierInfo[] = [
  {
    tier: 'new_user',
    label: 'New User',
    summary:
      'Default tier for newly registered accounts. You can apply existing tags, vote, submit shows, and propose edits to existing entities.',
    permissions: [
      'Apply existing tags to shows, artists, venues, and releases',
      'Vote on tags and content',
      'Submit shows for review',
      'Propose edits to existing entities (held for review)',
      'Comment and post field notes (held for review)',
    ],
  },
  {
    tier: 'contributor',
    label: 'Contributor',
    summary:
      'Trusted enough to create new canonical entities and tags directly, without review queues.',
    permissions: [
      'All New User permissions',
      'Create new tags directly from the Add Tag dialog',
      'Comments and field notes publish immediately',
    ],
    advancementFrom: 'new_user',
    advancementRequirements: [
      { text: '5 approved edits' },
      { text: 'Account age at least 14 days' },
      { text: 'Verified email address' },
    ],
  },
  {
    tier: 'trusted_contributor',
    label: 'Trusted Contributor',
    summary:
      'Proven accuracy and consistency. Direct edits bypass review and go live immediately.',
    permissions: [
      'All Contributor permissions',
      'Direct edits to existing entities without review',
      'Higher rate limits',
    ],
    advancementFrom: 'contributor',
    advancementRequirements: [
      { text: '25 approved edits' },
      { text: 'At least 95% approval rate' },
      { text: 'Account age at least 60 days' },
    ],
  },
  {
    tier: 'local_ambassador',
    label: 'Local Ambassador',
    summary:
      'Deep local expertise across venues and artists in a given scene. Highest trust tier.',
    permissions: [
      'All Trusted Contributor permissions',
      'Highest trust signal on the platform',
    ],
    advancementFrom: 'trusted_contributor',
    advancementRequirements: [
      { text: '50 approved edits' },
      { text: '10 approved edits on venues or artists' },
      { text: 'Account age at least 180 days' },
    ],
  },
]

export function getTierInfo(tier: UserTier): TierInfo {
  return TIERS.find(t => t.tier === tier) ?? TIERS[0]
}

export function getNextTierInfo(tier: UserTier): TierInfo | undefined {
  return TIERS.find(t => t.advancementFrom === tier)
}

export const TIERS_HELP_PATH = '/help/tiers'
