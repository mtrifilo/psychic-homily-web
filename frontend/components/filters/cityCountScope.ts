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
 * An unscoped call therefore sends the bare endpoint, which is the URL the
 * server-seeded first-screen payload is fetched with.
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
 * The cache-key half of the same contract: `undefined` for an empty scope, so an
 * unscoped facet keys on its base alone and the seeded entry matches.
 *
 * An empty tag list and the default tag match both normalize away, so one filter
 * state lands on one entry however a caller spelled it.
 */
export function cityCountScopeKey(
  scope: CityCountScope | undefined
): { tags: string[]; tagMatch?: 'any' } | undefined {
  if (!scope?.tags || scope.tags.length === 0) return undefined
  return scope.tagMatch === 'any'
    ? { tags: scope.tags, tagMatch: 'any' }
    : { tags: scope.tags }
}

/**
 * Appends a scope fragment to a facet's base query key, leaving the base
 * untouched when there is nothing to scope by. `extra` carries a surface's own
 * scope dimensions, which today is the /shows calendar window.
 *
 * Both halves of the key live here so a scoped request and the entry it lands in
 * cannot drift: react-query matches by the whole key, and a fragment added to
 * one side only is a cache that never hits.
 */
export function cityCountQueryKey(
  base: readonly unknown[],
  scope: CityCountScope | undefined,
  extra?: Record<string, unknown>
): readonly unknown[] {
  const key = cityCountScopeKey(scope)
  const fragment = { ...key, ...extra }
  // react-query hashes through JSON.stringify, which drops undefined members,
  // so a fragment whose every member is undefined is not a distinct entry; the
  // base has to be returned unchanged instead, or the seeded entry never hits.
  return Object.values(fragment).some(value => value !== undefined)
    ? [...base, fragment]
    : base
}

/**
 * A facet's request URL: the bare endpoint when there is nothing to scope by,
 * which is the request the server-seeded first-screen payload was fetched with.
 */
export function cityCountUrl(endpoint: string, params: URLSearchParams): string {
  const queryString = params.toString()
  return queryString ? `${endpoint}?${queryString}` : endpoint
}
