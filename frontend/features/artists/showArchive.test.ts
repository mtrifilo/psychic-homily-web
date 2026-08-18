import { describe, expect, it } from 'vitest'
import { artistArchiveHref, artistShowZone } from './showArchive'

/**
 * The artist archive's URL space, pinned in one place — the twin of
 * `features/venues/showArchive.test.ts`'s `venueArchiveHref` block (PSY-1842).
 *
 * These strings are what the year strip, both pagers and every "back to the
 * first page" link have to agree on, so a change here should be a deliberate
 * migration rather than a passing edit. `ArtistShowsList.test.tsx` exercises the
 * same rules through a rendered archive; this pins them at the function, which
 * is where the next caller (an artist year route, when one is built) will reach
 * them.
 */
describe('artistArchiveHref', () => {
  const base = { artistSlug: 'turnstile', artistId: 42, currentParams: new URLSearchParams() }

  it('addresses every year, page 1 as the bare artist page', () => {
    expect(artistArchiveHref({ ...base, year: null, page: 1 })).toBe(
      '/artists/turnstile#artist-past-shows'
    )
  })

  it('never emits ?page=1 — page 1 is the canonical bare URL for its scope', () => {
    expect(artistArchiveHref({ ...base, year: 2025, page: 1 })).toBe(
      '/artists/turnstile?year=2025#artist-past-shows'
    )
  })

  it('carries the year and the page together on a deep link', () => {
    expect(artistArchiveHref({ ...base, year: 2025, page: 3 })).toBe(
      '/artists/turnstile?year=2025&page=3#artist-past-shows'
    )
  })

  it('keeps the year OUT of the all-years address at any page', () => {
    expect(artistArchiveHref({ ...base, year: null, page: 2 })).toBe(
      '/artists/turnstile?page=2#artist-past-shows'
    )
  })

  it('carries a param this section does not own straight through', () => {
    // The connections graph pushes `?center=` onto the same URL and preserves
    // our two params in return. Building from an empty set would make that
    // courtesy one-way and silently drop the reader's graph center on every
    // page click.
    expect(
      artistArchiveHref({
        ...base,
        currentParams: new URLSearchParams('center=glass-harbor'),
        year: 2025,
        page: 2,
      })
    ).toBe('/artists/turnstile?center=glass-harbor&year=2025&page=2#artist-past-shows')
  })

  it('still drops its OWN stale params from the canonical page-1, all-years link', () => {
    expect(
      artistArchiveHref({
        ...base,
        currentParams: new URLSearchParams('year=2019&page=7&center=glass-harbor'),
        year: null,
        page: 1,
      })
    ).toBe('/artists/turnstile?center=glass-harbor#artist-past-shows')
  })

  it('falls back to the id for a slugless artist rather than linking the index', () => {
    // `GenerateSlug` returns "" for a name with no [a-z0-9] characters at all,
    // so a band called `!!!` reaches this page slugless. `/artists/` is the
    // artists INDEX, not a 404 — an unguarded href would eject the reader from
    // the archive entirely.
    expect(
      artistArchiveHref({ artistSlug: '', artistId: 42, currentParams: new URLSearchParams(), year: null, page: 1 })
    ).toBe('/artists/42#artist-past-shows')
  })
})

describe('artistShowZone', () => {
  it('reads a show on its own venue timezone, falling back to the venue state', () => {
    expect(artistShowZone({ venue: { state: 'IL', timezone: 'America/Chicago' } as never })).toEqual({
      state: 'IL',
      timezone: 'America/Chicago',
    })
  })

  it('resolves neither for a show with no venue at all', () => {
    // Documented downstream behaviour: `resolveShowTimezone` then applies its
    // silent America/Phoenix default. This pins that nothing here invents one.
    expect(artistShowZone({ venue: null })).toEqual({
      state: undefined,
      timezone: undefined,
    })
  })
})
