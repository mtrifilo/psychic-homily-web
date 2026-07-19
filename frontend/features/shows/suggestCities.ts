import { haversineDistanceKm } from '@/lib/haversine'
import type { CityState, CityWithCount } from '@/components/filters'

function cityKey(city: { city: string; state: string }): string {
  return `${city.city.trim().toLowerCase()}|${city.state.trim().toLowerCase()}`
}

/**
 * Pick alternative cities for a zero-result /shows filter (PSY-1433).
 *
 * Prefer nearest has-shows cities by haversine from the selected city's
 * centroid (already on `/shows/cities` via PSY-981). Fall back to most-active
 * by show_count when centroids are missing on either side.
 */
export function suggestAlternativeCities(
  cities: readonly CityWithCount[],
  selected: readonly CityState[],
  limit = 3
): CityWithCount[] {
  if (selected.length === 0 || limit <= 0) return []

  const selectedKeys = new Set(selected.map(cityKey))
  const candidates = cities.filter(city => !selectedKeys.has(cityKey(city)))
  if (candidates.length === 0) return []

  const anchors = selected
    .map(sel => cities.find(city => cityKey(city) === cityKey(sel)))
    .filter(
      (city): city is CityWithCount =>
        city != null &&
        typeof city.latitude === 'number' &&
        typeof city.longitude === 'number'
    )

  if (anchors.length > 0) {
    const withDistance = candidates
      .filter(
        city =>
          typeof city.latitude === 'number' &&
          typeof city.longitude === 'number'
      )
      .map(city => ({
        city,
        km: Math.min(
          ...anchors.map(anchor =>
            haversineDistanceKm(
              anchor.latitude!,
              anchor.longitude!,
              city.latitude!,
              city.longitude!
            )
          )
        ),
      }))
      .sort((a, b) => a.km - b.km || b.city.count - a.city.count)

    if (withDistance.length > 0) {
      return withDistance.slice(0, limit).map(entry => entry.city)
    }
  }

  return [...candidates].sort((a, b) => b.count - a.count).slice(0, limit)
}
