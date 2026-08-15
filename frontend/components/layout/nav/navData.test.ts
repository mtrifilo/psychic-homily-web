import { describe, it, expect } from 'vitest'
import {
  accountNavItems, navDestination, profileHref, sidebarAccountItems,
  sidebarGroups, visibleNavItems,
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

describe('visibleNavItems', () => {
  it('gates adminOnly entries on the viewer, one way for every surface', () => {
    for (const items of [accountNavItems(null), sidebarAccountItems(null)]) {
      expect(visibleNavItems(items, { is_admin: true }).map(i => i.label)).toContain('Admin')
      expect(visibleNavItems(items, { is_admin: false }).map(i => i.label)).not.toContain('Admin')
      expect(visibleNavItems(items, null).map(i => i.label)).not.toContain('Admin')
    }
  })
})

describe('sidebarGroups', () => {
  it('has Discover and Community groups', () => {
    expect(sidebarGroups.map(g => g.label)).toEqual(['Discover', 'Community'])
  })

  it('Discover keeps the rail order of catalog + discovery destinations', () => {
    const discover = sidebarGroups.find(g => g.label === 'Discover')!
    expect(discover.items.map(i => i.label)).toEqual([
      'Shows', 'Festivals', 'Artists', 'Venues', 'Graph', 'Releases', 'Labels',
      'Tags', 'Scenes', 'Atlas', 'Collections', 'Charts', 'Radio',
    ])
  })

  it('Community keeps the rail order of contribute + editorial destinations', () => {
    const community = sidebarGroups.find(g => g.label === 'Community')!
    expect(community.items.map(i => i.label)).toEqual([
      'Contribute', 'Leaderboard', 'Requests', 'Blog', 'DJ Sets', 'Substack',
      'Submit a Show', 'My Submissions',
    ])
  })

  it('only Substack is external', () => {
    const external = sidebarGroups.flatMap(g => g.items).filter(i => i.external)
    expect(external.map(i => i.label)).toEqual(['Substack'])
  })
})

describe('composition integrity', () => {
  it('every rail entry matches its canonical definition (modulo declared label overrides)', () => {
    for (const item of [...sidebarGroups.flatMap(g => g.items), ...sidebarAccountItems(null)]) {
      if (item.href === '/users/me') continue // Profile: href resolved per-user
      const canonical = navDestination(item.href)
      expect(item.icon, `${item.href} icon`).toBe(canonical.icon)
      expect(!!item.external, `${item.href} external`).toBe(!!canonical.external)
    }
  })

  it('no two destinations within a rail group share an icon (the double-Orbit defect class)', () => {
    const blocks = [...sidebarGroups.map(g => g.items), sidebarAccountItems(null)]
    for (const items of blocks) {
      const icons = items.map(i => i.icon)
      expect(new Set(icons).size, items.map(i => i.label).join(',')).toBe(icons.length)
    }
  })
})

describe('navDestination', () => {
  it('throws on an unknown href so a typo fails at module load, not as a dead link', () => {
    expect(() => navDestination('/no-such-route')).toThrow(/unknown nav destination/)
  })

  it('applies per-surface label overrides without touching the canonical entry', () => {
    expect(navDestination('/scenes', { label: 'Scenes' })).toMatchObject({
      href: '/scenes',
      label: 'Scenes',
    })
    expect(navDestination('/scenes').label).toBe('All scenes')
  })
})
