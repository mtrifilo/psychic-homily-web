/**
 * What a slug has to be before a URL can be built from it.
 *
 * Two callers need different amounts of this, which is why the rule is split
 * here rather than copied: a caller that ENCODES the value into a path needs
 * only the first check, because encoding gives it the segment guarantee for
 * free; a caller that interpolates a slug RAW into a URL has to test for that
 * guarantee itself.
 */

/**
 * Slug values that address an entity INDEX rather than an entity.
 *
 * Entity slugs are NULLABLE and `GenerateSlug` can return `""`, and `/artists/`
 * resolves to `/artists` rather than 404ing. `.` and `..` survive
 * `encodeURIComponent` untouched, so `/collections/..` walks back up to
 * `/collections`.
 */
const INDEX_ADDRESSING_SLUGS: ReadonlySet<string> = new Set(['', '.', '..'])

/** Does this slug address an entity rather than that entity type's index? */
export function addressesAnEntity(slug: string): boolean {
  return !INDEX_ADDRESSING_SLUGS.has(slug.trim())
}

/**
 * Characters that END or RE-TARGET a URL path segment.
 *
 * `/` and `\` open a second segment (browsers normalize the backslash), `?` and
 * `#` end the path, whitespace ends the URL in most parsers, and `%` is how an
 * encoded copy of any of the others reaches a decoder that has not run yet.
 */
const SEGMENT_BREAKING = /[/\\?#%\s]/

/**
 * Is this slug ALREADY one path segment, safe to interpolate without encoding?
 *
 * The question is whether the value stays inside the segment it is written
 * into, so the test is for the characters that would take it out of one.
 * `../..`, `phoenix-az?x` and `phoenix az` fail; `phoenix-az`, `st.-louis-mo`
 * and `española-nm` pass, and all three are slugs the backend's
 * `lower(replace(city,' ','-')) || '-' || lower(state)` rule can produce.
 *
 * Deliberately NOT `encodeURIComponent(slug) === slug`, which is the stricter
 * and more obvious spelling: it also rejects every non-ASCII character, so the
 * accented US city above would lose its page rather than a would-be attacker
 * losing a traversal.
 */
export function looksLikeSlug(slug: string): boolean {
  return addressesAnEntity(slug) && !SEGMENT_BREAKING.test(slug)
}
