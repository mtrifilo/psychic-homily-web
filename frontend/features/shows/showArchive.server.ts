/**
 * The archive derivations that only a SERVER render needs (PSY-1770).
 *
 * A separate module from `showArchive.ts` for one reason, and it is a
 * measured one rather than a stylistic one: this file imports `nuqs/server`,
 * and `showArchive.ts` is imported by six CLIENT modules — both archives'
 * tables and lists, and both entities' hooks. Putting a parser here rather
 * than there is what keeps a second copy of nuqs out of the venue and artist
 * page bundles.
 *
 * Measured with `bun build --target=browser --minify` on an entry importing
 * only `clampPage`: 24,576 bytes with the `nuqs/server` import in
 * `showArchive.ts`, 134 bytes without it. The client already ships `nuqs`
 * proper for `useQueryState`, so that 24 KB was a duplicate — an unusually
 * embarrassing thing for a performance ticket to add.
 *
 * Nothing here may be imported from a client component. The `.server` suffix
 * is the reminder; the import graph is the enforcement.
 */

import { parseAsInteger } from 'nuqs/server'
import { toPageNumber } from '@/components/shared/paginationChrome'

/**
 * The parser the archives read `?page=` with, on the SERVER.
 *
 * It has a twin: `VenuePastShows` calls `parseAsInteger.withDefault(1)` from the
 * `nuqs` client entry point, and the two must agree about what `?page=+2` or
 * `?page=2abc` names or the canonical page renders unseeded. They cannot be one
 * constant — `nuqs` and `nuqs/server` ship SEPARATE bundled copies of the same
 * runtime (verified in nuqs 2.9.0: `dist/index.js` and `dist/server.js` each
 * carry their own `createParser`), so a value from one does not satisfy the
 * other's `useQueryState`, and importing the client entry here is the very thing
 * this module exists to avoid. The agreement is pinned by the equivalence
 * battery in `showArchive.test.ts` rather than by construction. Change one side
 * and that test is what tells you.
 */
const archivePageParser = parseAsInteger.withDefault(1)

/**
 * Whether a URL is asking for the FIRST page of its archive.
 *
 * The one question a server render needs answered, and deliberately narrower
 * than "which page is this": page 1 is the only page the server has rows to
 * seed, so everything else is the same answer. Returning a boolean is what keeps
 * the per-surface page BOUND out of here — a maximum can only ever pull a number
 * down to itself, and every bound in use is far above 1, so it cannot change
 * whether the page is 1. Taking one as a parameter would imply a relevance it
 * does not have.
 *
 * BOTH steps of the client's derivation, not just the parser. `?page=0` and
 * `?page=-3` parse to 0 and -3 — perfectly good integers — and it is
 * {@link toPageNumber}, inside `clampPage`, that turns them into page 1 in the
 * browser. Testing the parsed value alone called those URLs "not page 1" and
 * withheld rows the client then asked page 1 for, which is the expensive
 * direction to be wrong in: it silently gives back the server-rendered archive
 * PSY-1756 bought. The equivalence battery is what caught it and what keeps it
 * caught.
 *
 * Entity-agnostic on purpose. The artist archive has no per-year route today, so
 * it does no server-side row seeding; when it gets one, this is the function it
 * must use rather than a third copy of the derivation.
 */
export function archiveIsFirstPage(
  searchParams: Record<string, string | string[] | undefined>
): boolean {
  const parsed = archivePageParser.parseServerSide(searchParams.page)
  return toPageNumber(parsed, 1) === 1
}
