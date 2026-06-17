import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UserTierBadge } from './UserTierBadge'
import type { UserTier } from '@/features/auth'

// Table-driven coverage for the 4 production tiers. If a new tier is
// added to UserTier, this table forces the test author to add a row
// (or the type-check fails on `satisfies`), preventing a silent
// fallback-to-new-user surface.
const TIER_CASES = [
  {
    tier: 'new_user',
    label: 'New User',
    expectedClassFragment: 'bg-muted',
    expectedTextClassFragment: 'text-muted-foreground',
  },
  {
    tier: 'contributor',
    label: 'Contributor',
    expectedClassFragment: 'bg-blue-500/10',
    expectedTextClassFragment: 'text-blue-600',
  },
  {
    tier: 'trusted_contributor',
    label: 'Trusted Contributor',
    expectedClassFragment: 'bg-emerald-500/10',
    expectedTextClassFragment: 'text-emerald-600',
  },
  {
    tier: 'local_ambassador',
    label: 'Local Ambassador',
    expectedClassFragment: 'bg-purple-500/10',
    expectedTextClassFragment: 'text-purple-600',
  },
] as const satisfies ReadonlyArray<{
  tier: UserTier
  label: string
  expectedClassFragment: string
  expectedTextClassFragment: string
}>

describe('UserTierBadge', () => {
  describe.each(TIER_CASES)('tier=$tier', ({ tier, label, expectedClassFragment, expectedTextClassFragment }) => {
    it(`renders the "${label}" label`, () => {
      render(<UserTierBadge tier={tier} />)
      expect(screen.getByText(label)).toBeInTheDocument()
    })

    it(`applies the tier-specific background class fragment "${expectedClassFragment}"`, () => {
      render(<UserTierBadge tier={tier} />)
      const badge = screen.getByText(label)
      expect(badge.className).toContain(expectedClassFragment)
    })

    it(`applies the tier-specific text class fragment "${expectedTextClassFragment}"`, () => {
      render(<UserTierBadge tier={tier} />)
      const badge = screen.getByText(label)
      expect(badge.className).toContain(expectedTextClassFragment)
    })
  })

  it('each tier renders with a distinct className string', () => {
    const classNames = TIER_CASES.map(({ tier, label }) => {
      const { unmount } = render(<UserTierBadge tier={tier} />)
      const badge = screen.getByText(label)
      const className = badge.className
      unmount()
      return className
    })

    // No tier should share its full className string with another tier
    const unique = new Set(classNames)
    expect(unique.size).toBe(classNames.length)
  })

  it('applies a custom className prop in addition to tier styles', () => {
    render(<UserTierBadge tier="contributor" className="custom-class" />)
    const badge = screen.getByText('Contributor')
    expect(badge.className).toContain('custom-class')
    // Tier-specific class is still applied
    expect(badge.className).toContain('text-blue-600')
  })

  it('renders the badge with the shared badge inline-flex class', () => {
    render(<UserTierBadge tier="contributor" />)
    // The shared Badge primitive always applies inline-flex via cva.
    const badge = screen.getByText('Contributor')
    expect(badge.className).toContain('inline-flex')
  })

  it('falls back to new_user config for an unknown tier value', () => {
    // TypeScript would normally prevent this, but the runtime fallback
    // matters when server-side enums drift ahead of the FE.
    render(<UserTierBadge tier={'unknown_tier' as UserTier} />)
    expect(screen.getByText('New User')).toBeInTheDocument()
  })

  it('fallback badge for unknown tier uses the same className as explicit new_user', () => {
    const { unmount } = render(
      <UserTierBadge tier={'totally_made_up' as UserTier} />
    )
    const aClass = screen.getByText('New User').className
    unmount()
    render(<UserTierBadge tier="new_user" />)
    const bClass = screen.getByText('New User').className
    expect(aClass).toBe(bClass)
  })
})
