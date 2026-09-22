/**
 * Saved-shows sizing shared by client hooks AND server helpers.
 *
 * Deliberately NOT in `hooks/useSavedShows.ts`: that module is `'use client'`,
 * and a value imported from it into the react-server layer arrives as a
 * client REFERENCE, not the number. `lib/auth-hydration.ts` reads this to
 * prefetch the signed-in home's rows, so it must live in a module with no
 * directive and no client-only imports.
 */

/**
 * How many saved shows a surface shows before sending the viewer to the full
 * list. Library's collapsed table, the signed-in home module, and the
 * infinite query's first page all read this one value.
 */
export const SAVED_SHOWS_COLLAPSED_COUNT = 4
