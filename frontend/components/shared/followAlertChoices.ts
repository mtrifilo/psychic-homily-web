import type {
  FollowAlertSettings,
  FollowAlertUpdate,
} from '@/lib/types/follow'

/**
 * The one vocabulary for a follow's new-show alert setting (PSY-1905).
 *
 * Two surfaces render this axis — the entity page's post-follow reveal and the
 * Library row's bracket menu — and both write the same field on the same
 * follow. Keeping the choices, their labels and the storage mapping here means
 * adding an option cannot land on one surface and miss the other.
 *
 * The stored shape is two independent fields (`enabled` plus, for artists,
 * `scope`); this collapses them into the single choice a person actually
 * makes. Release alerts are deliberately NOT on this axis: a record has no
 * location, so it has no scope, and its channels are an account-level setting.
 */
export type FollowAlertChoice = 'near_me' | 'everywhere' | 'off' | 'on'

/**
 * Capability-truth note, shared by every surface that offers this axis.
 *
 * The subscription is real and stored, but the delivery that acts on it is a
 * separate, unshipped piece of work. Controls may say what alerts WILL cover;
 * none of them may imply mail or feed entries are already flowing from it.
 * Delete this constant, and its call sites, when delivery ships.
 */
export const FOLLOW_ALERTS_PENDING_NOTE =
  'New-show alerts are still being switched on. This sets what they will cover once they are.'

export interface FollowAlertOption {
  value: FollowAlertChoice
  /** Chip label on the entity page. */
  label: string
  /** Lower-case form for the Library row's `[ alerts: … ]` bracket. */
  summary: string
}

const NEAR_ME: FollowAlertOption = {
  value: 'near_me',
  label: 'Near me',
  summary: 'near me',
}
const EVERYWHERE: FollowAlertOption = {
  value: 'everywhere',
  label: 'Everywhere',
  summary: 'everywhere',
}
const ON: FollowAlertOption = { value: 'on', label: 'On', summary: 'on' }
const OFF: FollowAlertOption = { value: 'off', label: 'Off', summary: 'off' }

/** Entity types whose follow carries an alert subscription, PLURAL as routed. */
export const ALERT_CAPABLE_FOLLOW_TYPES = ['artists', 'venues'] as const

export type AlertCapableFollowType =
  (typeof ALERT_CAPABLE_FOLLOW_TYPES)[number]

export const isAlertCapableFollowType = (
  entityType: string
): entityType is AlertCapableFollowType =>
  (ALERT_CAPABLE_FOLLOW_TYPES as readonly string[]).includes(entityType)

/**
 * The choices offered for one follow.
 *
 * A venue sits in one place, so its only axis is on or off. An artist tours,
 * so it gets a geographic scope — but "Near me" is withheld until a home area
 * exists, because a scoped subscription with nothing to scope to would look
 * configured and deliver nothing.
 */
export const followAlertOptions = ({
  entityType,
  hasHomeMetro,
}: {
  entityType: string
  hasHomeMetro: boolean
}): FollowAlertOption[] => {
  if (entityType === 'venues') return [ON, OFF]
  return hasHomeMetro ? [NEAR_ME, EVERYWHERE, OFF] : [EVERYWHERE, OFF]
}

/**
 * Which choice a resolved subscription currently represents.
 *
 * An artist follow whose stored scope is near-me while no home area exists
 * reads back as `everywhere`, matching what the server actually delivers: the
 * near-me fallback is applied at delivery time rather than baked into storage,
 * so the stored preference survives setting an area later.
 */
export const followAlertChoice = (
  settings: Pick<FollowAlertSettings, 'shows'> | undefined,
  { entityType, hasHomeMetro }: { entityType: string; hasHomeMetro: boolean }
): FollowAlertChoice | undefined => {
  if (!settings) return undefined
  if (!settings.shows.enabled) return 'off'
  if (entityType === 'venues') return 'on'
  return settings.shows.scope === 'near_me' && hasHomeMetro
    ? 'near_me'
    : 'everywhere'
}

/** The PATCH body for a choice. Only the axes the choice pins are sent. */
export const followAlertUpdateFor = (
  choice: FollowAlertChoice
): FollowAlertUpdate => {
  switch (choice) {
    case 'off':
      return { shows: { enabled: false } }
    case 'on':
      return { shows: { enabled: true } }
    case 'near_me':
      return { shows: { enabled: true, scope: 'near_me' } }
    case 'everywhere':
      return { shows: { enabled: true, scope: 'everywhere' } }
  }
}

/** The `[ alerts: … ]` bracket text for a Library row. */
export const followAlertSummary = (
  settings: Pick<FollowAlertSettings, 'shows'> | undefined,
  context: { entityType: string; hasHomeMetro: boolean }
): string | undefined => {
  const choice = followAlertChoice(settings, context)
  if (!choice) return undefined
  return [NEAR_ME, EVERYWHERE, ON, OFF].find(option => option.value === choice)
    ?.summary
}
