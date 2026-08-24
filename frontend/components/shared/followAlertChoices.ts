import type {
  FollowAlertPreference,
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

/**
 * `Object.hasOwn`, not a bare index. `entityType` is a plain string here, and
 * every object literal inherits `__proto__`, `constructor` and `toString`, so a
 * bare lookup can hand back an object or a function typed as `string`. Callers
 * render this straight into JSX. No call site can reach those keys today (all
 * three pass a closed literal set), which is exactly why an index would sit
 * here unnoticed until one of them started forwarding a route segment.
 */
export const followAlertPendingNote = (entityType: string): string | null =>
  Object.hasOwn(PENDING_DELIVERY_NOTES, entityType)
    ? PENDING_DELIVERY_NOTES[entityType]!
    : null

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

/**
 * The PATCH body for a choice. Only the axes the choice pins are sent.
 *
 * `hasHomeMetro` matters for exactly one case, and the read path is why. With
 * no home area, "Near me" is not offered, so "Everywhere" is not a scope the
 * user chose between: it is the only way to say ON, and `followAlertChoice`
 * already RELABELS a stored near-me scope as everywhere in that state. Pinning
 * `scope: 'everywhere'` from that click would overwrite the near-me preference
 * the read path goes out of its way to preserve, silently and permanently:
 * toggle a near-me follow off and back on while your area is unset, and setting
 * an area later no longer restores near me. Sending only `enabled` keeps the
 * promise `followAlertChoice`'s doc comment makes.
 */
export const followAlertUpdateFor = (
  choice: FollowAlertChoice,
  { hasHomeMetro }: { hasHomeMetro?: HomeMetroState } = {}
): FollowAlertUpdate => {
  switch (choice) {
    case 'off':
      return { shows: { enabled: false } }
    case 'on':
      return { shows: { enabled: true } }
    case 'near_me':
      return { shows: { enabled: true, scope: 'near_me' } }
    case 'everywhere':
      return hasHomeMetro === false
        ? { shows: { enabled: true } }
        : { shows: { enabled: true, scope: 'everywhere' } }
  }
}

/**
 * Whether an enabled subscription has any channel to arrive on.
 *
 * `enabled` is only half of the promise. Channels are an ACCOUNT-level setting
 * (the alert matrix), so a person who switches both in-app and email off for
 * new-show alerts leaves every follow's subscription enabled and delivering
 * nothing. The notifier says exactly this and skips the recipient outright:
 * `if !pref.Enabled || (!pref.InApp && !pref.Email) { continue }`
 * (`backend/internal/services/notification/artist_follow_notify.go`).
 *
 * The controls therefore cannot read `enabled` alone and call it active. A
 * chip reading "Near me" over a subscription in that state is a delivery
 * promise nothing behind it can keep.
 */
export const followAlertHasChannel = (
  preference: Pick<FollowAlertPreference, 'in_app' | 'email'>
): boolean => preference.in_app || preference.email

/**
 * Whether this follow's show alerts are ON but reaching nobody: PAUSED.
 *
 * A distinct third state, not a synonym for off. "Off" is a choice made on
 * THIS follow and its scope has been given up; paused is an account-wide
 * silence sitting on top of a follow whose scope is still stored and still
 * meant. Collapsing the two would have the surfaces write `enabled: false`
 * (or read back as if someone had), losing a preference the person never
 * changed the moment they switch a channel off.
 *
 * Which is why every paused surface points at the alert matrix rather than
 * offering a fix of its own: the channel is the thing that is off, and it is
 * not a per-follow field.
 */
export const followAlertsPaused = (
  settings: Pick<FollowAlertSettings, 'shows'> | undefined
): boolean =>
  settings !== undefined &&
  settings.shows.enabled &&
  !followAlertHasChannel(settings.shows)

/** The bracket/summary word for a paused subscription, in both surfaces. */
export const ALERTS_PAUSED_SUMMARY = 'paused'

/**
 * Why a follow reads paused, and what un-pauses it.
 *
 * One string because the entity page and the Library row must not explain the
 * same state two ways, and because the reassurance is load-bearing: without
 * it, "paused" reads as "your setting was discarded" and the honest fix looks
 * like re-picking a scope that was never lost.
 */
export const ALERTS_PAUSED_NOTE =
  'New-show alerts are paused: in-app and email are both switched off for them in your alert settings. Your scope for this follow is saved and resumes when you switch a channel back on.'

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
