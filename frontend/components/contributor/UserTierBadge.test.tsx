import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UserTierBadge } from './UserTierBadge'
import type { UserTier } from '@/features/auth'

describe('UserTierBadge', () => {
  it('renders "New User" badge for new_user tier', () => {
    render(<UserTierBadge tier="new_user" />)
    expect(screen.getByText('New User')).toBeInTheDocument()
  })

  it('renders "Contributor" badge for contributor tier', () => {
    render(<UserTierBadge tier="contributor" />)
    expect(screen.getByText('Contributor')).toBeInTheDocument()
  })

  it('renders "Trusted Contributor" badge for trusted_contributor tier', () => {
    render(<UserTierBadge tier="trusted_contributor" />)
    expect(screen.getByText('Trusted Contributor')).toBeInTheDocument()
  })

  it('renders "Local Ambassador" badge for local_ambassador tier', () => {
    render(<UserTierBadge tier="local_ambassador" />)
    expect(screen.getByText('Local Ambassador')).toBeInTheDocument()
  })

  it('applies tier-specific classes for contributor', () => {
    render(<UserTierBadge tier="contributor" />)
    const badge = screen.getByText('Contributor')
    expect(badge.className).toContain('text-blue-600')
  })

  it('applies tier-specific classes for trusted_contributor', () => {
    render(<UserTierBadge tier="trusted_contributor" />)
    const badge = screen.getByText('Trusted Contributor')
    expect(badge.className).toContain('text-emerald-600')
  })

  it('applies tier-specific classes for local_ambassador', () => {
    render(<UserTierBadge tier="local_ambassador" />)
    const badge = screen.getByText('Local Ambassador')
    expect(badge.className).toContain('text-purple-600')
  })

  it('applies custom className', () => {
    render(<UserTierBadge tier="contributor" className="custom-class" />)
    const badge = screen.getByText('Contributor')
    expect(badge.className).toContain('custom-class')
  })

  it('falls back to new_user config for unknown tier', () => {
    // TypeScript would normally prevent this, but test runtime behavior
    render(<UserTierBadge tier={'unknown_tier' as UserTier} />)
    expect(screen.getByText('New User')).toBeInTheDocument()
  })
})
