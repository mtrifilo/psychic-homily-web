/**
 * The `/settings` hub's section registry.
 *
 * The anchors are a redirect CONTRACT, not presentation: every link into the
 * hub is `/settings#<anchor>`, and there are no `/settings/<section>`
 * subroutes. A renamed anchor degrades every such link to a silent scroll to
 * the top, so anchors are spelled once, here, and every caller builds its href
 * with {@link settingsSectionHref}.
 *
 * ISOMORPHIC ON PURPOSE: no `'use client'`, and nothing here may import a
 * client module, so a server module (a redirect, a prefetch) can read these
 * values rather than a client reference to them.
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

export const SETTINGS_HREF = '/settings'

export function settingsSectionHref(anchor: SettingsAnchor): string {
  return `${SETTINGS_HREF}#${anchor}`
}

/** The profile editor's tabs, where most controls still live. */
const PROFILE_EDITOR_HREF = '/profile'
const PROFILE_PRIVACY_HREF = '/profile?tab=privacy'
const PROFILE_SECTIONS_HREF = '/profile?tab=sections'
const PROFILE_SETTINGS_HREF = '/profile?tab=settings'
const APPEARANCE_SETTINGS_HREF = '/settings/appearance'

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

const MANAGE_IN_PROFILE_SETTINGS = 'Manage in profile settings'

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
        href: PROFILE_SETTINGS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
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
        href: PROFILE_EDITOR_HREF,
        action: 'Manage in profile editor',
      },
      {
        kind: 'link',
        covers: ['Custom sections'],
        href: PROFILE_SECTIONS_HREF,
        action: 'Manage in profile sections',
      },
      {
        kind: 'link',
        covers: ['Who sees what'],
        href: PROFILE_PRIVACY_HREF,
        action: 'Manage in profile privacy',
      },
      {
        kind: 'link',
        covers: ['Default reply permission'],
        href: PROFILE_SETTINGS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
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
        href: PROFILE_SETTINGS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
      },
      {
        kind: 'link',
        covers: ['Your area'],
        href: ALERTS_AREA_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
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
        href: ALERTS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
      },
      {
        kind: 'link',
        covers: ['Custom alerts'],
        href: CUSTOM_ALERTS_HREF,
        action: 'Manage in custom alerts',
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
        href: APPEARANCE_SETTINGS_HREF,
        action: 'Manage in appearance settings',
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
        href: PROFILE_SETTINGS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
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
        href: PROFILE_PRIVACY_HREF,
        action: 'Manage in profile privacy',
      },
      {
        kind: 'link',
        covers: ['Export or delete your account'],
        href: PROFILE_SETTINGS_HREF,
        action: MANAGE_IN_PROFILE_SETTINGS,
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
