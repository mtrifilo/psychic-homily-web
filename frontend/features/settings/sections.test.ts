import { describe, it, expect } from 'vitest'
import {
  ALERTS_ANCHOR,
  ALERTS_AREA_ANCHOR,
} from '@/components/shared/followAlertChoices'
import {
  SETTINGS_ANCHORS,
  SETTINGS_SECTIONS,
  settingsSectionCount,
  settingsSectionHref,
  type SettingsLinkRow,
} from './sections'

const linkRows = (): SettingsLinkRow[] =>
  SETTINGS_SECTIONS.flatMap(section =>
    section.rows.filter((row): row is SettingsLinkRow => row.kind === 'link')
  )

describe('SETTINGS_SECTIONS', () => {
  it('lists the seven sections in the blessed order with the blessed titles', () => {
    expect(SETTINGS_SECTIONS.map(section => section.title)).toEqual([
      'Account',
      'Public profile',
      'Home page',
      'Alerts and email',
      'Appearance',
      'Feeds',
      'Privacy and data',
    ])
  })

  // The anchors are the redirect contract every later settings link is
  // written against; changing one is a breaking change, not a refactor.
  it('pins every anchor the hub answers to', () => {
    expect(SETTINGS_ANCHORS).toEqual({
      account: 'account',
      publicProfile: 'public-profile',
      homePage: 'home-page',
      alerts: 'alerts',
      alertsArea: 'alerts-area',
      appearance: 'appearance',
      feeds: 'feeds',
      privacy: 'privacy',
    })
    expect(SETTINGS_SECTIONS.map(section => section.anchor)).toEqual([
      'account',
      'public-profile',
      'home-page',
      'alerts',
      'appearance',
      'feeds',
      'privacy',
    ])
  })

  it('shares the alert anchors with the account alert matrix', () => {
    expect(ALERTS_ANCHOR).toBe('alerts')
    expect(ALERTS_AREA_ANCHOR).toBe('alerts-area')
    expect(SETTINGS_ANCHORS.alerts).toBe(ALERTS_ANCHOR)
    expect(SETTINGS_ANCHORS.alertsArea).toBe(ALERTS_AREA_ANCHOR)
  })

  it('gives every fragment exactly one element', () => {
    const fragments = [
      ...SETTINGS_SECTIONS.map(section => section.anchor),
      ...linkRows().flatMap(row => (row.anchor ? [row.anchor] : [])),
    ]
    expect(new Set(fragments).size).toBe(fragments.length)
    expect([...fragments].sort()).toEqual(
      Object.values(SETTINGS_ANCHORS).sort()
    )
  })

  it('puts "Your area" inside the Home page section', () => {
    const homePage = SETTINGS_SECTIONS.find(
      section => section.anchor === SETTINGS_ANCHORS.homePage
    )
    const areaRow = homePage?.rows.find(
      (row): row is SettingsLinkRow =>
        row.kind === 'link' && row.anchor === SETTINGS_ANCHORS.alertsArea
    )
    expect(areaRow?.covers).toEqual(['Your area'])
  })

  it('mounts the live home layout list only in the Home page section', () => {
    const owners = SETTINGS_SECTIONS.filter(section =>
      section.rows.some(row => row.kind === 'home-layout')
    ).map(section => section.anchor)
    expect(owners).toEqual([SETTINGS_ANCHORS.homePage])
  })

  it('links every row to a surface that exists today, never back into the hub', () => {
    const existing = [
      /^\/profile$/,
      /^\/profile\?tab=(privacy|sections|settings)(#[a-z-]+)?$/,
      /^\/settings\/(appearance|notification-filters)$/,
    ]
    for (const row of linkRows()) {
      expect(existing.some(pattern => pattern.test(row.href))).toBe(true)
      expect(row.href.startsWith('/settings#')).toBe(false)
      expect(row.covers.length).toBeGreaterThan(0)
      expect(row.action).toMatch(/^Manage in /)
    }
  })
})

describe('settingsSectionCount', () => {
  it('counts the settings each section names', () => {
    expect(
      Object.fromEntries(
        SETTINGS_SECTIONS.map(section => [
          section.anchor,
          settingsSectionCount(section),
        ])
      )
    ).toEqual({
      account: 6,
      'public-profile': 4,
      'home-page': 3,
      alerts: 4,
      appearance: 1,
      feeds: 2,
      privacy: 2,
    })
  })
})

describe('settingsSectionHref', () => {
  it('addresses a section as a fragment on the one hub route', () => {
    expect(settingsSectionHref(SETTINGS_ANCHORS.account)).toBe(
      '/settings#account'
    )
    expect(settingsSectionHref(SETTINGS_ANCHORS.alertsArea)).toBe(
      '/settings#alerts-area'
    )
  })
})
