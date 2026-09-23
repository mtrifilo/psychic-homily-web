/**
 * The `/settings` hub's section registry.
 *
 * The anchors are a redirect CONTRACT, not presentation: a link into the hub
 * is `/settings#<anchor>`, and there are no `/settings/<section>` subroutes. A
 * renamed anchor degrades every such link to a silent scroll to the top, so
 * anchors are spelled once, here.
 *
 * ISOMORPHIC ON PURPOSE: no `'use client'`, and nothing here may import a
 * client module, so a server module can read these values rather than a
 * client reference to them.
 */

import {
  ALERTS_ANCHOR,
  ALERTS_AREA_ANCHOR,
  ALERTS_HREF,
  ALERTS_AREA_HREF,
  CUSTOM_ALERTS_HREF,
} from '@/components/shared/followAlertChoices'

/**
 * Every fragment the hub answers to. `alertsArea` is not a section of its own:
 * it names the "Your area" row inside the Home page section. Its value is the
 * one the account alert matrix already uses, so a link written against either
 * surface keeps the same fragment.
 */
export const SETTINGS_ANCHORS = {
  account: 'account',
  publicProfile: 'public-profile',
  homePage: 'home-page',
  alerts: ALERTS_ANCHOR,
  alertsArea: ALERTS_AREA_ANCHOR,
  appearance: 'appearance',
  feeds: 'feeds',
  privacy: 'privacy',
} as const

export type SettingsAnchor =
  (typeof SETTINGS_ANCHORS)[keyof typeof SETTINGS_ANCHORS]

/**
 * Where controls live today, each with the action label that names it. A row
 * spreads one of these, so a label cannot drift from its destination.
 */
const PROFILE_EDITOR = { href: '/profile', action: 'Manage in profile editor' }
const PROFILE_PRIVACY = {
  href: '/profile?tab=privacy',
  action: 'Manage in profile privacy',
}
const PROFILE_SECTIONS = {
  href: '/profile?tab=sections',
  action: 'Manage in profile sections',
}
const PROFILE_SETTINGS = {
  href: '/profile?tab=settings',
  action: 'Manage in profile settings',
}
const PROFILE_ALERTS = { href: ALERTS_HREF, action: PROFILE_SETTINGS.action }
const PROFILE_ALERTS_AREA = {
  href: ALERTS_AREA_HREF,
  action: PROFILE_SETTINGS.action,
}
const CUSTOM_ALERTS = {
  href: CUSTOM_ALERTS_HREF,
  action: 'Manage in custom alerts',
}
const APPEARANCE_SETTINGS = {
  href: '/settings/appearance',
  action: 'Manage in appearance settings',
}

/**
 * A row that sends the viewer to the surface that owns these settings today.
 * `covers` names the settings the destination holds; the rail's per-section
 * count is the number of names, so it cannot drift from what the rows say.
 */
export interface SettingsLinkRow {
  kind: 'link'
  covers: readonly string[]
  href: string
  action: string
  /** A fragment inside the hub that lands on this row rather than on its
   *  section. */
  anchor?: SettingsAnchor
}

/** The live show / hide / reorder list for the signed-in home page. */
export interface SettingsHomeLayoutRow {
  kind: 'home-layout'
}

export type SettingsRow = SettingsLinkRow | SettingsHomeLayoutRow

export interface SettingsSection {
  anchor: SettingsAnchor
  title: string
  blurb?: string
  rows: readonly SettingsRow[]
}

/** Rail, jump index and page order. */
export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  {
    anchor: SETTINGS_ANCHORS.account,
    title: 'Account',
    blurb:
      'Who you are to the system: how you sign in, and how you leave. Nothing here is public.',
    rows: [
      {
        kind: 'link',
        covers: [
          'Email address',
          'Password',
          'Passkeys',
          'Connected accounts',
          'Export your data',
          'Delete account',
        ],
        ...PROFILE_SETTINGS,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.publicProfile,
    title: 'Public profile',
    rows: [
      {
        kind: 'link',
        covers: ['Identity and bio'],
        ...PROFILE_EDITOR,
      },
      {
        kind: 'link',
        covers: ['Custom sections'],
        ...PROFILE_SECTIONS,
      },
      {
        kind: 'link',
        covers: ['Who sees what'],
        ...PROFILE_PRIVACY,
      },
      {
        kind: 'link',
        covers: ['Default reply permission'],
        ...PROFILE_SETTINGS,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.homePage,
    title: 'Home page',
    blurb:
      'Show, hide, and reorder the sections on your home page. Saved on your account.',
    rows: [
      { kind: 'home-layout' },
      {
        kind: 'link',
        covers: ['Favorite cities'],
        ...PROFILE_SETTINGS,
      },
      {
        kind: 'link',
        covers: ['Your area'],
        ...PROFILE_ALERTS_AREA,
        anchor: SETTINGS_ANCHORS.alertsArea,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.alerts,
    title: 'Alerts and email',
    rows: [
      {
        kind: 'link',
        covers: ['Alert matrix', 'Reminders and digests', 'Account emails'],
        ...PROFILE_ALERTS,
      },
      {
        kind: 'link',
        covers: ['Custom alerts'],
        ...CUSTOM_ALERTS,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.appearance,
    title: 'Appearance',
    blurb: 'How the site looks and how the chrome is laid out.',
    rows: [
      {
        kind: 'link',
        covers: ['Navigation style'],
        ...APPEARANCE_SETTINGS,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.feeds,
    title: 'Feeds',
    rows: [
      {
        kind: 'link',
        covers: ['Calendar feed', 'Follows activity feed'],
        ...PROFILE_SETTINGS,
      },
    ],
  },
  {
    anchor: SETTINGS_ANCHORS.privacy,
    title: 'Privacy and data',
    rows: [
      {
        kind: 'link',
        covers: ['Profile visibility'],
        ...PROFILE_PRIVACY,
      },
      {
        kind: 'link',
        covers: ['Export or delete your account'],
        ...PROFILE_SETTINGS,
      },
    ],
  },
]

/** How many settings a section holds, as its rows name them. */
export function settingsSectionCount(section: SettingsSection): number {
  return section.rows.reduce(
    (total, row) => total + (row.kind === 'link' ? row.covers.length : 1),
    0
  )
}
