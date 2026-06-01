'use client'

import type { CityState } from './CityFilters'

/**
 * The overridable "Showing {City}, {ST} (from your location) — change" chip
 * shown under the city filter when a selection was auto-applied from the IP-geo
 * default (PSY-926 /explore, PSY-946 /shows + home). Rendered identically on
 * all three surfaces; the parent decides WHEN via `shouldShowGeoAffordance` and
 * supplies the override handler.
 */
export function GeoDefaultAffordance({
  city,
  onChange,
}: {
  city: CityState
  onChange: () => void
}) {
  return (
    <p
      className="mt-1.5 text-xs text-muted-foreground"
      data-testid="geo-default-affordance"
    >
      Showing {city.city}, {city.state} (from your location) —{' '}
      <button
        type="button"
        onClick={onChange}
        className="text-primary hover:underline underline-offset-4"
        data-testid="geo-default-change"
      >
        change
      </button>
    </p>
  )
}
