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
 * Does this slug address an entity rather than that entity type's index?
 *
 * Three values do not. Entity slugs are NULLABLE and `GenerateSlug` can return
 * `""`, and `/artists/` resolves to `/artists` rather than 404ing; `.` and `..`
 * survive `encodeURIComponent` untouched, so `/collections/..` walks back up to
 * `/collections`. Trimmed first, because whitespace around any of the three
 * addresses the index just as surely.
 */
export function addressesAnEntity(slug: string): boolean {
  const trimmed = slug.trim()
  return trimmed !== '' && trimmed !== '.' && trimmed !== '..'
}

/**
 * Characters that END or RE-TARGET a URL path segment.
 *
 * `/` and `\` open a second segment (browsers normalize the backslash), `?` and
 * `#` end the path, and `%` is how an encoded copy of any of the others reaches
 * a decoder that has not run yet.
 *
 * Whitespace is NOT here. Surrounding space is handled below, where it names a
 * different address; space INSIDE a slug does not take the value out of its
 * segment, and the backend's slug rule only replaces U+0020, so a city carrying
 * a non-breaking space produces a slug that addresses a page this site serves.
 */
const SEGMENT_BREAKING = /[/\\?#%]/

/**
 * Is this slug ALREADY one path segment, safe to interpolate without encoding?
 *
 * Two questions in one. The value must stay inside the segment it is written
 * into, so the characters that would take it out are refused; and it must be
 * the slug it appears to be, so a leading or trailing space is refused too,
 * because `" phoenix-az"` and `"phoenix-az"` are different addresses and only
 * one of them is a page.
 *
 * `../..`, `phoenix-az?x` and `" phoenix-az"` fail; `phoenix-az`,
 * `st.-louis-mo` and `española-nm` pass, and all three are slugs the backend's
 * `lower(replace(city,' ','-')) || '-' || lower(state)` rule can produce.
 *
 * Deliberately NOT `encodeURIComponent(slug) === slug`, which is the stricter
 * and more obvious spelling: it also rejects every non-ASCII character, so the
 * accented US city above would lose its page rather than a would-be attacker
 * losing a traversal.
 */
export function looksLikeSlug(slug: string): boolean {
  return slug === slug.trim() && !SEGMENT_BREAKING.test(slug) && addressesAnEntity(slug)
}
