import { describe, it, expect } from 'vitest'
import { Bell, Library } from 'lucide-react'
import {
  PROFILE_CLAIM_HREF, accountNavItems, buildDestinationIndex, mobileNavFor,
  navDestination, profileHref, sidebarAccountItems, sidebarGroups, visibleNavItems,
} from './navData'
import type { MobileNavViewer } from './navData'

describe('profileHref', () => {
  it('deep-links to the public identity view when the user has a username', () => {
    expect(profileHref({ username: 'reggie' })).toBe('/users/reggie')
  })

  it('routes to the claim-username self view without one', () => {
    expect(profileHref({})).toBe(PROFILE_CLAIM_HREF)
    expect(profileHref(null)).toBe(PROFILE_CLAIM_HREF)
  })
})

describe('accountNavItems', () => {
  it('is the canonical account set, in menu order, with Admin flagged last', () => {
    const items = accountNavItems(null)
    expect(items.map(i => i.label)).toEqual([
      'Notifications', 'Library', 'Profile', 'Settings', 'Appearance', 'Admin',
    ])
    // Admin last is load-bearing: UserMenu renders the adminOnly partition
    // after a separator while the mobile sheet renders in list order — a
    // non-admin entry after Admin would order differently per surface.
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

describe('mobileNavFor', () => {
  const viewers: MobileNavViewer[] = ['signed-in', 'signed-out']
  const labels = (viewer: MobileNavViewer) =>
    mobileNavFor(viewer).primaryTabs.map(t => t.label)
  const discover = (viewer: MobileNavViewer) =>
    mobileNavFor(viewer).browseGroups.find(g => g.label === 'Discover')!.items.map(i => i.label)

  // Product Designs P1a (2076:26 / 2077:22): Library takes Radio's tab once
  // signed in; everyone else keeps the bar as it was.
  it('gives a signed-in viewer Library in the third tab and everyone else Radio', () => {
    expect(labels('signed-in')).toEqual(['Home', 'Shows', 'Library'])
    expect(labels('signed-out')).toEqual(['Home', 'Shows', 'Radio'])
    expect(mobileNavFor('signed-in').primaryTabs[2].href).toBe('/library')
  })

  it('leads the Discover group with Radio only for the viewer whose bar dropped it', () => {
    expect(discover('signed-in')).toEqual(['Radio', 'Graph', 'Atlas'])
    expect(discover('signed-out')).toEqual(['Graph', 'Atlas'])
  })

  // The bar renders a literal grid-cols-5; a viewer with a different tab
  // count would wrap a tab outside the fixed bar height.
  it('keeps the tab count the same for every viewer', () => {
    const counts = viewers.map(v => mobileNavFor(v).primaryTabs.length)
    expect(new Set(counts).size).toBe(1)
  })

  it('never lists a destination in both the bar and the Browse sheet', () => {
    for (const viewer of viewers) {
      const { primaryTabs, browseHrefs } = mobileNavFor(viewer)
      for (const tab of primaryTabs) expect(browseHrefs, viewer).not.toContain(tab.href)
    }
  })

  it('resolves every tab through the canonical tables', () => {
    for (const viewer of viewers) {
      for (const tab of mobileNavFor(viewer).primaryTabs) {
        expect(tab).toEqual(navDestination(tab.href))
      }
    }
  })
})

describe('visibleNavItems', () => {
  const admin = { email: 'a@test.com', is_admin: true }
  const member = { email: 'm@test.com', is_admin: false }

  it('gates adminOnly entries on the viewer, one way for every surface', () => {
    for (const items of [accountNavItems(null), sidebarAccountItems(null)]) {
      expect(visibleNavItems(items, admin).map(i => i.label)).toContain('Admin')
      expect(visibleNavItems(items, member).map(i => i.label)).not.toContain('Admin')
      expect(visibleNavItems(items, null).map(i => i.label)).not.toContain('Admin')
    }
  })

  it('gates authOnly entries on any signed-in viewer', () => {
    const community = sidebarGroups.find(g => g.label === 'Community')!
    expect(visibleNavItems(community.items, null).map(i => i.label)).not.toContain(
      'My Submissions'
    )
    expect(visibleNavItems(community.items, member).map(i => i.label)).toContain(
      'My Submissions'
    )
  })
})

describe('buildDestinationIndex', () => {
  const entry = { href: '/x', label: 'X', icon: Bell }

  it('collapses equal duplicates', () => {
    expect(buildDestinationIndex([[entry], [{ ...entry }]]).size).toBe(1)
  })

  it('throws on conflicting duplicates, gating flags included', () => {
    expect(() =>
      buildDestinationIndex([[entry], [{ ...entry, label: 'Y' }]])
    ).toThrow(/conflicting definitions/)
    expect(() =>
      buildDestinationIndex([[entry], [{ ...entry, icon: Library }]])
    ).toThrow(/conflicting definitions/)
    expect(() =>
      buildDestinationIndex([[entry], [{ ...entry, authOnly: true }]])
    ).toThrow(/conflicting definitions/)
    expect(() =>
      buildDestinationIndex([[entry], [{ ...entry, adminOnly: true }]])
    ).toThrow(/conflicting definitions/)
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
      expect(item.href, `${item.href} shape`).toMatch(/^(\/|https?:\/\/)/)
      if (item.href === PROFILE_CLAIM_HREF) continue // Profile: href resolved per-user
      const canonical = navDestination(item.href)
      expect(item.icon, `${item.href} icon`).toBe(canonical.icon)
      expect(!!item.external, `${item.href} external`).toBe(!!canonical.external)
    }
  })

  it('no two destinations within a rendered group share an icon (the double-Orbit defect class)', () => {
    // The mobile Scenes group is excluded: its city entries share MapPin on
    // purpose (same icon = same kind of destination).
    const blocks = [
      ...sidebarGroups.map(g => g.items),
      sidebarAccountItems(null),
      accountNavItems(null),
      ...(['signed-in', 'signed-out'] as const).flatMap(viewer =>
        mobileNavFor(viewer).browseGroups.filter(g => g.label !== 'Scenes').map(g => g.items)
      ),
    ]
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
    // Asserts the mechanism only — deliberately independent of what the
    // canonical copy actually says, so a pure copy edit can't fail it.
    const canonical = navDestination('/scenes').label
    expect(navDestination('/scenes', { label: 'Zzz' }).label).toBe('Zzz')
    expect(navDestination('/scenes').label).toBe(canonical)
  })
})
