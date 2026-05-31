import { describe, it, expect } from 'vitest'
import {
  adminNavGroups,
  VALID_TABS,
  isValidTab,
  adminTabHref,
  isAdminTabActive,
} from './adminNav'

// Guards the single-source-of-truth coupling (PSY-933): the admin nav is now the
// ONLY way into the /admin tab content, so a section in VALID_TABS with no nav
// item is unreachable, and a nav item with a stray tab lands on a dead section.
// `tab: AdminTab` makes a bad literal a compile error; this makes the
// coverage/uniqueness a test error.
describe('adminNav', () => {
  const navTabs = adminNavGroups.flatMap(g => g.items.map(i => i.tab))

  it('covers exactly VALID_TABS — no missing or extra sections', () => {
    expect([...navTabs].sort()).toEqual([...VALID_TABS].sort())
  })

  it('lists every section exactly once (no duplicate nav items)', () => {
    expect(new Set(navTabs).size).toBe(navTabs.length)
  })

  it('only the four queue sections carry a badge key', () => {
    const badged = adminNavGroups
      .flatMap(g => g.items)
      .filter(i => i.badgeKey)
      .map(i => i.tab)
    expect(badged.sort()).toEqual(
      ['moderation', 'pending-shows', 'reports', 'unverified-venues'].sort()
    )
  })

  it('adminTabHref: dashboard is bare /admin, others carry ?tab=', () => {
    expect(adminTabHref('dashboard')).toBe('/admin')
    expect(adminTabHref('moderation')).toBe('/admin?tab=moderation')
  })

  it('isValidTab accepts known tabs and rejects unknown / null', () => {
    expect(isValidTab('moderation')).toBe(true)
    expect(isValidTab('pending-venue-edits')).toBe(false) // a known stale value
    expect(isValidTab(null)).toBe(false)
  })

  it('isAdminTabActive: dashboard active at bare /admin; sections match ?tab=', () => {
    expect(isAdminTabActive('dashboard', '/admin', null)).toBe(true)
    expect(isAdminTabActive('dashboard', '/admin', 'moderation')).toBe(false)
    expect(isAdminTabActive('moderation', '/admin', 'moderation')).toBe(true)
    expect(isAdminTabActive('reports', '/admin', 'moderation')).toBe(false)
    expect(isAdminTabActive('moderation', '/shows', null)).toBe(false)
  })
})
