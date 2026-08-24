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
 * Capability truth, per alert type, because they no longer share a state.
 *
 * ARTIST show alerts DELIVER as of PSY-1896: the matcher runs inside
 * MatchAndNotify and sends both the in-app row and the email. Nothing here may
 * still call those "coming soon".
 *
 * Venue show alerts and release alerts have a stored, resolved subscription and
 * no delivery behind it: there is no venue-follow notifier and no release
 * notifier. Their controls may say what the alerts WILL cover; they may not
 * imply anything is already flowing. Delete each note as its delivery lands.
 */
export const VENUE_ALERTS_PENDING_NOTE =
  'Alerts for shows a venue you follow adds are still being switched on. This setting decides what they will cover once they are.'

export const RELEASE_ALERTS_PENDING_NOTE =
  'Release alerts are still being switched on. These settings decide where they will reach you once they are.'

/**
 * Where a viewer with no home area goes to set one, and where an alert email's
 * "manage" link lands.
 *
 * The anchors and the hrefs live together because they are one fact. Split
 * across the linking component and the linked card, renaming an anchor
 * degrades the link to a silent scroll-to-top that no test would catch.
 */
export const ALERTS_ANCHOR = 'alerts'
export const ALERTS_HREF = `/profile?tab=settings#${ALERTS_ANCHOR}`
export const ALERTS_AREA_ANCHOR = 'alerts-area'
export const ALERTS_AREA_HREF = `/profile?tab=settings#${ALERTS_AREA_ANCHOR}`

/** The Custom alerts manager, still routed under its original path. */
export const CUSTOM_ALERTS_HREF = '/settings/notification-filters'

export interface FollowAlertOption {
  value: FollowAlertChoice
  /** Chip label on the entity page. */
  label: string
}

const NEAR_ME: FollowAlertOption = { value: 'near_me', label: 'Near me' }
const EVERYWHERE: FollowAlertOption = {
  value: 'everywhere',
  label: 'Everywhere',
}
const ON: FollowAlertOption = { value: 'on', label: 'On' }
const OFF: FollowAlertOption = { value: 'off', label: 'Off' }

/**
 * Whether a follow type carries an alert subscription at all, PLURAL as routed.
 *
 * The server is the authority here: its alert endpoints 422 for every other
 * follow type, and it signals capability per row by populating (or omitting)
 * `alerts`. Prefer that signal where a payload is in hand. This predicate is
 * for the ONE case with no payload to read: an entity page deciding whether to
 * ask in the first place.
 */
export const isAlertCapableFollowType = (entityType: string): boolean =>
  entityType === 'artists' || entityType === 'venues'

/**
 * Whether this follow type's alerts have a geographic scope at all.
 *
 * A venue sits in one place. Anything that talks about "near me" has to ask
 * this first, or it ends up explaining a restriction the user does not have.
 */
export const followAlertHasScopeAxis = (entityType: string): boolean =>
  entityType !== 'venues'

/**
 * The pending-delivery disclosure for one follow type, or null when that
 * type's alerts genuinely deliver today.
 *
 * Delivery status is its OWN fact, listed per type rather than inferred from
 * the scope axis. Inferring it read as a coincidence that happened to hold
 * today (the type that tours is the one PSY-1896 shipped delivery for) and
 * would have silently granted "delivers" to any alert type added later.
 *
 * When venue delivery lands (PSY-1895), delete its entry here and every
 * surface stops disclosing it. That is a deliberate edit, not something this
 * function discovers on its own.
 */
const PENDING_DELIVERY_NOTES: Record<string, string> = {
  venues: VENUE_ALERTS_PENDING_NOTE,
}

export const followAlertPendingNote = (entityType: string): string | null =>
  PENDING_DELIVERY_NOTES[entityType] ?? null

/**
 * Whether the viewer has a home area, or `undefined` while that is still
 * UNKNOWN (the preferences query in flight, or failed).
 *
 * Unknown has to be representable and distinct from `false`. Collapsing the
 * two makes a loading page assert "you have no home area": it hides the Near
 * me chip a viewer already qualifies for, relabels their near-me follows as
 * "everywhere", and offers them a link to set an area they set months ago.
 */
export type HomeMetroState = boolean | undefined

interface FollowAlertContext {
  entityType: string
  hasHomeMetro: HomeMetroState
}

/**
 * The choices offered for one follow, or `undefined` while the home area is
 * still unknown.
 *
 * A venue sits in one place, so its only axis is on or off and it never has to
 * wait. An artist tours, so it gets a geographic scope, and "Near me" is
 * withheld until an area exists: a scoped subscription with nothing to scope
 * to would look configured and deliver nothing.
 */
export const followAlertOptions = ({
  entityType,
  hasHomeMetro,
}: FollowAlertContext): FollowAlertOption[] | undefined => {
  if (!followAlertHasScopeAxis(entityType)) return [ON, OFF]
  if (hasHomeMetro === undefined) return undefined
  return hasHomeMetro ? [NEAR_ME, EVERYWHERE, OFF] : [EVERYWHERE, OFF]
}

/**
 * Which choice a resolved subscription currently represents, or `undefined`
 * when either the subscription or the home area is still unknown.
 *
 * An artist follow whose stored scope is near-me while no home area exists
 * reads back as `everywhere`, matching what the server actually delivers: the
 * near-me fallback is applied at delivery time rather than baked into storage,
 * so the stored preference survives setting an area later. That relabelling is
 * only honest once the area is KNOWN to be absent, which is why the unknown
 * case returns undefined instead of falling through to it.
 */
export const followAlertChoice = (
  settings: Pick<FollowAlertSettings, 'shows'> | undefined,
  { entityType, hasHomeMetro }: FollowAlertContext
): FollowAlertChoice | undefined => {
  if (!settings) return undefined
  if (!settings.shows.enabled) return 'off'
  if (!followAlertHasScopeAxis(entityType)) return 'on'
  if (hasHomeMetro === undefined) return undefined
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

/**
 * The `[ alerts: … ]` bracket text for a Library row.
 *
 * Derived from the chip label rather than stored beside it: a second string
 * per option is a second thing to keep in step for no gain, since the bracket
 * form has always been the label in lower case.
 */
export const followAlertSummaryFor = (
  options: FollowAlertOption[],
  choice: FollowAlertChoice
): string | undefined =>
  options.find(option => option.value === choice)?.label.toLowerCase()
