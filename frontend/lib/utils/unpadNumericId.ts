/**
 * Drops the leading zeros of an all-digit id or slug (`'0042'` -> `'42'`);
 * anything that is not all digits passes unchanged.
 *
 * The backend reads an all-digit path segment as a numeric id and parses it
 * with leading zeros allowed, so every padding names the same row. Normalizing
 * before a server-side cached fetch keeps those paddings on ONE Data Cache entry
 * instead of one per spelling.
 */
export function unpadNumericId(idOrSlug: string): string {
  return /^\d+$/.test(idOrSlug) ? idOrSlug.replace(/^0+(?=\d)/, '') : idOrSlug
}
