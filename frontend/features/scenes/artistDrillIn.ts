/**
 * Pure logic for the Atlas artist drill-in (PSY-1541) — the travel-mode payoff:
 * land in an unfamiliar city, pick a venue, and hear every band playing there
 * this week without leaving the map.
 *
 * Kept free of React (like cityView.ts) so the two rules that actually carry
 * risk — how a bill becomes an ordered, de-duplicated step list, and what the
 * stepper claims about your position in it — unit-test without a DOM.
 *
 * Every "the mock" below means the approved board 03 "Artist drill-in" on the
 * Atlas page of the Product Designs Figma file (node 1154:6).
 */

import type { ArtistGraphCard } from '@/features/artists/types'
import type { VenueShow } from '@/features/venues/types'

// ── The step list ─────────────────────────────────────────────────────────

/**
 * One artist in the drill-in's stepping order.
 *
 * A flattened view of the list the user drilled in FROM, not of any one show:
 * the `‹ ›` stepper walks the whole originating list (a locked user decision,
 * 2026-07-25), so a venue drill-in steps that venue's week and a future
 * citywide surface steps citywide. `showId` records which show contributed the
 * artist, so the panel can say where the step came from without re-searching.
 */
export interface ArtistStep {
  artistId: number
  /** May be empty — the backend serves `""` for an artist with no slug yet. */
  artistSlug: string
  artistName: string
  /** The show this artist was first seen on, in the originating list's order. */
  showId: number
}

/**
 * Flatten an ordered show list into the artist list the stepper walks.
 *
 * Order is the list's own: shows in the order given (the venue-shows endpoint
 * already sorts soonest-first), and within a show the bill in `position` order
 * as served. That makes "2 of 5" mean the same thing as reading the panel's
 * show rows top to bottom, which is the only ordering a user can predict.
 *
 * De-duplicated by artist id, FIRST occurrence winning: a band playing the
 * venue twice this week is one entry, at its soonest date. Without this the
 * stepper would revisit the same artist mid-walk and the "of N" total would
 * count a band twice.
 *
 * Artists without a usable id are dropped rather than stepped to: the panel's
 * every fetch keys on that id, so an id-less entry would be a step onto a
 * permanently loading card.
 */
export function buildArtistSteps(
  shows: readonly VenueShow[] | null | undefined,
): ArtistStep[] {
  const steps: ArtistStep[] = []
  const seen = new Set<number>()
  // `?? []` at both levels, deliberately. `/atlas` has no route-level error
  // boundary, so a TypeError on an absent bill takes down the whole app shell
  // rather than just this panel — the same exposure VenuePanel's show-title
  // fallback guards, and the one PSY-1540's review found unguarded.
  for (const show of shows ?? []) {
    for (const artist of show?.artists ?? []) {
      if (!artist || typeof artist.id !== 'number' || artist.id <= 0) continue
      if (seen.has(artist.id)) continue
      seen.add(artist.id)
      steps.push({
        artistId: artist.id,
        artistSlug: artist.slug ?? '',
        artistName: artist.name,
        showId: show.id,
      })
    }
  }
  return steps
}

/**
 * Where to start stepping when a given show's row was clicked: the first
 * artist on THAT SHOW'S BILL, as the row displays it.
 *
 * Resolved against the bill rather than against `showId`, and the difference
 * is load-bearing wherever a band plays the venue twice. De-duplication
 * attributes such a band to its FIRST date, so a `showId` match would skip
 * straight past it on the second date's row — click "night two", whose bill
 * reads "Meat Wave / Gouge Away", and land on Gouge Away. The user clicked a
 * row and must land on the top of the bill they read.
 *
 * Returns -1 when the show contributed no steppable artist (an empty or
 * entirely id-less bill), which the caller must treat as "nothing to drill
 * into" rather than as index 0 — opening on a different show's headliner
 * because the clicked one had no bill is worse than not opening.
 */
export function firstStepIndexForShow(
  steps: readonly ArtistStep[],
  show: Pick<VenueShow, 'artists'> | null | undefined,
): number {
  for (const artist of show?.artists ?? []) {
    if (!artist) continue
    const index = steps.findIndex((step) => step.artistId === artist.id)
    if (index >= 0) return index
  }
  return -1
}

/** Clamp an index into the list, so an out-of-range step can't blank the panel. */
export function clampStepIndex(
  steps: readonly ArtistStep[],
  index: number,
): number {
  if (steps.length === 0) return 0
  if (!Number.isFinite(index)) return 0
  return Math.min(Math.max(Math.trunc(index), 0), steps.length - 1)
}

// ── Stepper copy ──────────────────────────────────────────────────────────

/**
 * The mock's mono kicker: "ARTIST · 2 OF 5 THIS WEEK AT THIS VENUE".
 *
 * `scopeLabel` is a PROP all the way up rather than the literal the mock draws,
 * because the locked decision is that the stepper's scope is whatever list you
 * drilled in from. Baking "at this venue" in here would be the one thing the
 * decision rules out.
 *
 * Returns just "ARTIST" for a single-entry list — a "1 OF 1" stepper is noise.
 */
export function artistStepKicker(args: {
  index: number
  total: number
  scopeLabel: string
}): string {
  const { index, total, scopeLabel } = args
  if (total <= 1) return 'ARTIST'
  const scope = scopeLabel.trim()
  const position = `${index + 1} OF ${total}`
  return scope
    ? `ARTIST · ${position} ${scope.toUpperCase()}`
    : `ARTIST · ${position}`
}

/**
 * The same position as a SENTENCE, for the panel's live region.
 *
 * The visible kicker is abbreviated mono caps and the `‹ ›` glyphs imply the
 * position visually; neither states it to a screen reader, and stepping does
 * not move focus (the buttons stay put on purpose), so without an announcement
 * a non-sighted user gets no feedback that anything changed. Names the artist
 * too — "2 of 5" alone doesn't say who you landed on.
 */
export function artistStepAnnouncement(args: {
  index: number
  total: number
  artistName: string
  scopeLabel: string
}): string {
  const { index, total, artistName, scopeLabel } = args
  if (total <= 1) return artistName
  // Verbatim, NOT case-folded: the scope carries a proper noun (a venue or
  // city name), and lowercasing it to match the sentence's register turns
  // "Hotel Vegas" into "hotel vegas" in a screen reader's ear. The kicker's
  // own uppercasing is safe because it uppercases everything.
  const scope = scopeLabel.trim()
  const where = scope ? ` ${scope}` : ''
  return `${artistName}, artist ${index + 1} of ${total}${where}`
}

// ── Panel copy ────────────────────────────────────────────────────────────

/**
 * The mock's CONNECTIONS line: "14 bills · 6 similar artists · Sub Pop ·
 * plays on WFMU".
 *
 * Same source fields as ArtistContextPanel's connections row (they describe
 * the same artist), but spelled out rather than abbreviated: this panel has
 * one line for all of it where the graph card has a labelled row per kind, so
 * "6 similar" would lose the noun. Empty segments are dropped, and an artist
 * with nothing to say returns '' so the caller omits the heading entirely.
 *
 * Labels and stations are capped so the line stays a GLANCE rather than a
 * paragraph — an artist on eight compilations would otherwise list all eight.
 * The cap bounds the open-ended segments only; a maximally-connected artist
 * still renders six segments and is allowed to wrap to a second line, which is
 * why the caller does not clamp it. Truncating mid-name would be worse than
 * wrapping: these are the connections the panel exists to advertise.
 */
export function artistConnectionsLine(card: ArtistGraphCard): string {
  const parts: string[] = []
  const connections = card.connections
  if (connections) {
    if (connections.bills > 0) {
      parts.push(`${connections.bills} ${connections.bills === 1 ? 'bill' : 'bills'}`)
    }
    if (connections.similar > 0) {
      parts.push(
        `${connections.similar} similar ${connections.similar === 1 ? 'artist' : 'artists'}`,
      )
    }
    if (connections.members > 0) {
      parts.push(
        `${connections.members} ${connections.members === 1 ? 'member' : 'members'}`,
      )
    }
  }
  const labels = (card.labels ?? []).slice(0, 2).map((label) => label.name)
  parts.push(...labels)
  const stations = (card.radio?.stations ?? []).slice(0, 2)
  if (stations.length > 0) {
    parts.push(`plays on ${stations.join(' & ')}`)
  }
  return parts.join(' · ')
}

/**
 * The mock's artist identity line: "Austin, TX".
 *
 * The mock also draws a genre family ("punk & hardcore") and an ACTIVE chip.
 * Neither is served on ANY artist contract — `ArtistGraphCard` carries no
 * genre rollup and no activity flag, and the only `is_active` in the codebase
 * is the SCENE ROSTER's (contracts.SceneArtistResponse), computed per-scene
 * over a metro roster and not reachable from an artist id. Deriving them means
 * extending the graph-card contract with an entity_tags rollup and an
 * active-window query — a backend change with its own tests, tracked
 * separately rather than guessed at here. Same call, for the same reason, as
 * VenuePanel omitting the mock's neighborhood and contributor counts: a
 * fabricated genre on an artist page is worse than a shorter line.
 */
export function artistIdentityLine(
  card: Pick<ArtistGraphCard, 'city' | 'state'>,
): string {
  return [card.city, card.state].filter(Boolean).join(', ')
}
