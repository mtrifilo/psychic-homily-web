/**
 * The timezone the server renders the first screen in, before it knows the
 * viewer's own (PSY-1624).
 *
 * `GET /shows/upcoming` uses its `timezone` parameter for exactly one thing:
 * placing the start-of-today boundary that decides which shows still count as
 * upcoming. Its documented default is UTC, and UTC is the wrong canonical for
 * this audience.
 *
 * WHY NOT UTC. The boundary is the most recent local midnight in the chosen
 * zone. UTC's sits about seven hours behind Pacific local midnight, so for most
 * of the day a UTC-rendered list re-admits the PREVIOUS evening's shows.
 * Measured against production on 2026-08-01 at 18:40Z: `timezone=UTC` led with
 * four shows dated `2026-08-01T00:00Z`, which is 5 PM Pacific the previous
 * calendar day and long finished, where `America/Los_Angeles` correctly led
 * with that night's `2026-08-02T00:00Z` shows. Start times cluster at 00:00 to
 * 03:00Z, so this is the common case rather than an edge.
 *
 * WHY IT IS SAFE FOR VIEWERS IN OTHER ZONES. Stated as the invariant, because
 * the intuitive version ("pick the latest US boundary") is both wrong and
 * false: Honolulu and Anchorage are later than Pacific. The invariant is that a
 * show is dropped only when it starts before the canonical midnight, and the
 * canonical midnight is by definition not in the future. Everything this
 * boundary excludes has therefore ALREADY STARTED. It cannot hide a show that
 * is still upcoming, from any viewer anywhere. The opposite case is the benign
 * one: where the canonical boundary falls earlier than the viewer's own, the
 * first screen carries a few extra already-started rows and the client's
 * refetch trims them.
 *
 * So the choice is not about correctness, which holds for any zone. It is about
 * how much finished content a JS-less fetcher is shown, and a Pacific boundary
 * trims nearly all of it for a US audience. Viewers in HST or AKST see slightly
 * more stale rows than a Pacific viewer, never fewer real ones.
 *
 * This is a stopgap for a modelling problem, not a preference. "Upcoming"
 * should be decided per show against its VENUE's zone, which is how the cards
 * already render their times, and needs no viewer timezone at all. PSY-1678
 * tracks that change and retires this constant along with the parameter.
 *
 * No `'use client'`: both the client hook that reports it and the
 * server-importable query descriptors keyed on it read this one value, and
 * they have to agree byte for byte or the seeded cache entry is not the entry
 * the hook looks up.
 */
export const CANONICAL_FIRST_SCREEN_TIMEZONE = 'America/Los_Angeles'
