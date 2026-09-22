/**
 * The signed-in home's section registry and the pure layout algebra over it.
 *
 * ISOMORPHIC ON PURPOSE: no `'use client'` here, and nothing in this file may
 * import a client module. `SignedInHome` is a server component, and a runtime
 * value imported from a `'use client'` module arrives in a server module as a
 * client REFERENCE (a function) rather than the value. Component identity
 * therefore stays out of this file; only ids, copy, the default order and pure
 * functions live here.
 *
 * The id set and the document shape are DERIVED from the generated API
 * contract, not restated: the backend owns the whitelist, and a hand-written
 * union would let a sixth id ship on the backend and be silently dropped here
 * (see resolveHomeLayout) instead of failing the build. The order, the copy and
 * the default visibility are this module's own; they are not in the contract.
 */

import type { components } from '@/types/api'

/** The only document version this reader understands. */
const HOME_LAYOUT_VERSION = 1

/** The Settings card that mirrors the home popover, and the route to it.
 *  Declared here rather than beside the card so the popover can link to it
 *  without features/home depending on features/auth. */
export const HOME_LAYOUT_SETTINGS_ANCHOR = 'home-layout'
export const HOME_LAYOUT_SETTINGS_HREF = `/profile?tab=settings#${HOME_LAYOUT_SETTINGS_ANCHOR}`

export type HomeSectionId =
  components['schemas']['HomeLayoutSection']['id']

export interface HomeSectionDefinition {
  id: HomeSectionId
  /** Row label in the customize list. Static: the list must name the same
   *  section on every render, and the graph section's own heading names a city
   *  that rotates under "Surprise me". */
  title: string
  description: string
  defaultVisible: boolean
}

/**
 * Default order and default visibility, as shipped.
 *
 * Array position IS the default slot: a section a stored document omits (a
 * section shipped after that document was written) is restored at its index
 * here, not appended at the end.
 */
export const HOME_SECTIONS: readonly HomeSectionDefinition[] = [
  {
    id: 'saved_shows',
    title: 'Your upcoming shows',
    description: 'Shows you saved, soonest first.',
    defaultVisible: true,
  },
  {
    id: 'nearby_shows',
    title: 'Shows near you this week',
    description: 'Upcoming shows near you that you have not saved yet.',
    defaultVisible: true,
  },
  {
    id: 'community_stats',
    title: 'Community stats',
    description: 'Shows in the next 7 days and entities in the graph.',
    defaultVisible: true,
  },
  {
    id: 'city_graph',
    title: 'Scene graph',
    description: 'The scene, mapped. Click a node to explore.',
    defaultVisible: true,
  },
  {
    id: 'radio_shows',
    title: 'Latest radio shows',
    description: 'Live freeform stations and what they are playing.',
    defaultVisible: true,
  },
]

const SECTIONS_BY_ID = new Map<HomeSectionId, HomeSectionDefinition>(
  HOME_SECTIONS.map(section => [section.id, section])
)

export type HomeLayoutSectionEntry = components['schemas']['HomeLayoutSection']

/** The stored document, exactly as `user_preferences.home_layout` holds it.
 *  `$schema` is a response-only field the client never sends. */
export type HomeLayoutDocument = Omit<
  components['schemas']['HomeLayout'],
  '$schema'
>

/** A registry entry with this viewer's visibility applied, in render order. */
export interface ResolvedHomeSection extends HomeSectionDefinition {
  visible: boolean
}

/**
 * Whether an unknown value is a document this reader understands. The version
 * check lives here rather than at each boundary so a v2 document is refused in
 * one place instead of half-accepted at the edge and discarded later.
 */
export function isHomeLayoutDocument(
  value: unknown
): value is HomeLayoutDocument {
  if (!value || typeof value !== 'object') return false
  const document = value as HomeLayoutDocument
  return (
    document.version === HOME_LAYOUT_VERSION &&
    (document.sections === null || Array.isArray(document.sections))
  )
}

function defaultSections(): ResolvedHomeSection[] {
  return HOME_SECTIONS.map(section => ({
    ...section,
    visible: section.defaultVisible,
  }))
}

/**
 * Merge a stored document with the registry into the order the page renders.
 *
 * Three rules, each of which a viewer would notice if it were dropped:
 *   - Ids the registry no longer knows are DROPPED. A retired section must not
 *     hold a slot nobody can fill.
 *   - Known ids the document omits are restored AT THEIR DEFAULT INDEX, so a
 *     section shipped after the viewer last saved lands where everyone else
 *     sees it instead of at the bottom.
 *   - A document whose version this reader does not understand is ignored
 *     entirely. Guessing at unknown semantics is worse than the default.
 *
 * Duplicate ids keep their first occurrence: array order is placement, and a
 * second entry for the same section has no placement to give.
 */
export function resolveHomeLayout(
  document: HomeLayoutDocument | null | undefined
): ResolvedHomeSection[] {
  if (!isHomeLayoutDocument(document)) return defaultSections()
  const stored = document.sections
  if (!stored || stored.length === 0) return defaultSections()

  const seen = new Set<HomeSectionId>()
  const resolved: ResolvedHomeSection[] = []
  for (const entry of stored) {
    if (!entry || seen.has(entry.id)) continue
    const definition = SECTIONS_BY_ID.get(entry.id)
    // An id the registry no longer knows is dropped here, by the lookup.
    if (!definition) continue
    seen.add(entry.id)
    resolved.push({ ...definition, visible: entry.visible !== false })
  }

  if (resolved.length === 0) return defaultSections()

  // Restore omitted sections at their registry index. Walking the registry in
  // order keeps two newcomers in their own relative order too.
  HOME_SECTIONS.forEach((definition, index) => {
    if (seen.has(definition.id)) return
    resolved.splice(Math.min(index, resolved.length), 0, {
      ...definition,
      visible: definition.defaultVisible,
    })
  })

  return resolved
}

/** The document to PUT for a resolved list. Always stamped with the version
 *  this module writes, never the one it read. */
export function toHomeLayoutDocument(
  sections: readonly ResolvedHomeSection[]
): HomeLayoutDocument {
  return {
    version: HOME_LAYOUT_VERSION,
    sections: sections.map(({ id, visible }) => ({ id, visible })),
  }
}

/**
 * Swap one section with its neighbour, or `null` when the row is already at
 * that end. Callers distinguish the two: only a real move is worth animating,
 * announcing and persisting.
 */
export function moveHomeSection(
  sections: readonly ResolvedHomeSection[],
  id: HomeSectionId,
  direction: 'up' | 'down'
): ResolvedHomeSection[] | null {
  const from = sections.findIndex(section => section.id === id)
  if (from === -1) return null
  const to = direction === 'up' ? from - 1 : from + 1
  if (to < 0 || to >= sections.length) return null
  const next = [...sections]
  ;[next[from], next[to]] = [next[to], next[from]]
  return next
}

/** Toggle visibility in place. Position never changes: a hidden section keeps
 *  its slot so re-showing puts it back where the viewer left it. */
export function setHomeSectionVisibility(
  sections: readonly ResolvedHomeSection[],
  id: HomeSectionId,
  visible: boolean
): ResolvedHomeSection[] {
  return sections.map(section =>
    section.id === id ? { ...section, visible } : section
  )
}

/** Whether this list is the shipped layout, so the reset control can say
 *  whether it has anything to do. */
export function isDefaultHomeLayout(
  sections: readonly ResolvedHomeSection[]
): boolean {
  return (
    sections.length === HOME_SECTIONS.length &&
    sections.every(
      (section, index) =>
        section.id === HOME_SECTIONS[index].id &&
        section.visible === HOME_SECTIONS[index].defaultVisible
    )
  )
}

/**
 * Where the "All upcoming shows in {city} →" link renders for a given layout.
 *
 * The link lives on the nearby section's header. Hiding that section would
 * take the only path from the home page to the full show list with it, so it
 * falls back to the saved-shows footer and then to the toolbar row, which is
 * always present for a signed-in viewer.
 */
export type HomeCityLinkSlot = 'nearby' | 'saved' | 'toolbar'

export function resolveCityLinkSlot(
  sections: readonly ResolvedHomeSection[]
): HomeCityLinkSlot {
  const isVisible = (id: HomeSectionId) =>
    sections.some(section => section.id === id && section.visible)
  if (isVisible('nearby_shows')) return 'nearby'
  if (isVisible('saved_shows')) return 'saved'
  return 'toolbar'
}
