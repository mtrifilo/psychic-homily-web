/**
 * The filters a city-count facet is scoped by, shared by /venues, /shows and
 * /artists.
 *
 * A facet counted on a wider set than the list it filters puts a number on the
 * bottom sheet's apply button that the page below it then contradicts, so the
 * counts have to be requested under the filters the list is reading. Only the
 * non-place half travels: the response IS the per-place breakdown, so narrowing
 * it to a place would leave the picker offering only the place already picked.
 *
 * The window half of the /shows scope is not here. It is spelled by
 * `appendShowsCalendarWindow` / `showsCalendarWindowKey`, which the shows list
 * already builds its own request from, and restating it would be a second
 * spelling of one contract.
 */
export interface CityCountScope {
  /** Tag slugs, applied with AND unless tagMatch says otherwise. */
  tags?: string[]
  /** 'any' switches the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
}

/**
 * The request half: appends the scope's params, and nothing at all when the
 * scope is empty.
 *
 * An unscoped call therefore sends the bare endpoint, byte for byte the request
 * this facet made before it took filters, which is what keeps the server-seeded
 * first-screen payload usable.
 */
export function appendCityCountScope(
  params: URLSearchParams,
  scope: CityCountScope | undefined
): void {
  if (!scope?.tags || scope.tags.length === 0) return
  params.set('tags', scope.tags.join(','))
  if (scope.tagMatch === 'any') params.set('tag_match', 'any')
}

/**
 * The cache-key half of the same contract: `undefined` for an empty scope, so
 * the unscoped facet keeps the exact key it had before, and the seeded entry
 * still matches.
 *
 * An empty tag list and the default tag match both normalize away, so one filter
 * state lands on one entry however a caller spelled it.
 */
export function cityCountScopeKey(
  scope: CityCountScope | undefined
): { tags: string[]; tagMatch?: 'any' } | undefined {
  if (!scope?.tags || scope.tags.length === 0) return undefined
  return {
    tags: scope.tags,
    tagMatch: scope.tagMatch === 'any' ? 'any' : undefined,
  }
}

/**
 * Appends a scope fragment to a facet's base query key, leaving the base
 * untouched when there is nothing to scope by.
 *
 * Both halves of the key live here so a scoped request and the entry it lands in
 * cannot drift: react-query matches by the whole key, and a fragment added to
 * one side only is a cache that never hits.
 */
export function cityCountQueryKey(
  base: readonly unknown[],
  scope: CityCountScope | undefined
): readonly unknown[] {
  const key = cityCountScopeKey(scope)
  return key ? [...base, key] : base
}
