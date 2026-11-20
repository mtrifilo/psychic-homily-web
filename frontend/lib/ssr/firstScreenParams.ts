/**
 * The URL half of a seeded list's first screen, shared by the server seed gate
 * (`dehydrateListFirstScreen`) and the browser fallback that must agree with it
 * (`CachedListFallback`).
 *
 * No directive and no server import: a client component imports this module,
 * and both sides have to apply the same rule to the same URL.
 */

/** A route's `searchParams`, as Next hands them to a page. */
export type RouteSearchParams = Record<string, string | string[] | undefined>

/**
 * Whether a URL's params keep it on a list's seeded first screen: true when
 * none of `defeatingParams` carries a non-empty value.
 *
 * The test is for a non-empty value, not for a value that moves the client
 * hook's cache key, so a param restating the default (`?page=1`) still skips a
 * seed it would have hit. That is the safe direction: a false positive costs a
 * server-rendered loader, while a false negative would seed a key the hook
 * never reads.
 *
 * Empty values (`?tags=`, `?page=`) read as absent, because the list
 * components parse them as absent. A repeated param is an array; any non-empty
 * member counts.
 */
export function isFirstScreenRequest(
  params: RouteSearchParams,
  defeatingParams: readonly string[],
): boolean {
  return !defeatingParams.some(key => {
    const value = params[key]
    if (Array.isArray(value)) return value.some(v => v !== '')
    return value !== undefined && value !== ''
  })
}

/**
 * The `defeatingParams` entries of a browser URL, in the shape Next hands a
 * page, so `isFirstScreenRequest` reads a client URL exactly as the server
 * read the same request.
 */
export function routeSearchParamsOf(
  searchParams: { getAll(name: string): string[] },
  names: readonly string[],
): RouteSearchParams {
  return Object.fromEntries(
    names.map(name => {
      const values = searchParams.getAll(name)
      return [name, values.length <= 1 ? values[0] : values]
    }),
  )
}
