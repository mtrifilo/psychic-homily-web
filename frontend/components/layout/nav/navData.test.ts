import { describe, it, expect } from 'vitest'
import {
  accountNavItems, atlasItem, graphItem, navDestination, profileHref,
  sidebarAccountItems, sidebarGroups,
} from './navData'

describe('profileHref', () => {
  it('deep-links to the public identity view when the user has a username', () => {
    expect(profileHref({ username: 'reggie' })).toBe('/users/reggie')
  })

  it('routes to the claim-username self view without one', () => {
    expect(profileHref({})).toBe('/users/me')
    expect(profileHref(null)).toBe('/users/me')
  })
})

describe('accountNavItems', () => {
  it('is the canonical account set, in menu order, with Admin flagged last', () => {
    const items = accountNavItems(null)
    expect(items.map(i => i.label)).toEqual([
      'Notifications', 'My Library', 'Profile', 'Settings', 'Appearance', 'Admin',
    ])
    expect(items.at(-1)?.adminOnly).toBe(true)
    expect(items.filter(i => i.adminOnly)).toHaveLength(1)
  })

  it('resolves the Profile href through the username rule', () => {
    const profile = accountNavItems({ username: 'reggie' }).find(i => i.label === 'Profile')
    expect(profile?.href).toBe('/users/reggie')
  })

  it('keeps Notification Filters desktop-only (PSY-1821 decision)', () => {
    // The rail carries it; the account menus do not — mobile reaches it via
    // the link on /notifications. Changing this is a product decision, not a
    // refactor.
    const hrefs = accountNavItems(null).map(i => i.href)
    expect(hrefs).not.toContain('/settings/notification-filters')
    expect(sidebarAccountItems(null).map(i => i.href)).toContain(
      '/settings/notification-filters'
    )
  })
})

describe('sidebarAccountItems', () => {
  it('includes Admin only for admins', () => {
    expect(sidebarAccountItems({ is_admin: true }).map(i => i.label)).toContain('Admin')
    expect(sidebarAccountItems({ is_admin: false }).map(i => i.label)).not.toContain('Admin')
  })

  it('uses the canonical My Library label (rail used to fork it as "Library")', () => {
    expect(sidebarAccountItems(null).map(i => i.label)).toContain('My Library')
  })
})

describe('navDestination', () => {
  it('throws on an unknown href so a typo fails at module load, not as a dead link', () => {
    expect(() => navDestination('/no-such-route')).toThrow(/unknown nav destination/)
  })

  it('resolves canonical entries and applies per-surface label overrides', () => {
    expect(navDestination('/artists').label).toBe('Artists')
    expect(navDestination('/scenes', { label: 'Scenes' })).toMatchObject({
      href: '/scenes',
      label: 'Scenes',
    })
  })
})

describe('one icon per destination', () => {
  it('gives Graph and Atlas distinct canonical icons (the rail double-Orbit fix)', () => {
    expect(graphItem.icon).not.toBe(atlasItem.icon)
  })

  it('the rail renders the canonical Graph/Atlas entries, not forks', () => {
    const discover = sidebarGroups.find(g => g.label === 'Discover')!
    expect(discover.items.find(i => i.href === '/graph')?.icon).toBe(graphItem.icon)
    expect(discover.items.find(i => i.href === '/atlas')?.icon).toBe(atlasItem.icon)
  })
})
