'use client'

import { useMemo } from 'react'
import { Loader2 } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
// Subpath, not the feature barrel: Turbopack does not tree-shake a
// `'use client'` barrel per-export, so importing one from a widely-reachable
// shared component drags the whole charts surface into the global shared
// chunk (PSY-1772).
import { useChartScenes } from '@/features/charts/hooks/useCharts'
import { useSetHomeMetro } from '@/features/auth/hooks/useAlertPreferences'
import { cn } from '@/lib/utils'

/**
 * The metro list offered as a home area.
 *
 * `all_time` rather than a rolling window on purpose: this is a standing
 * preference, and a metro that happened to book nothing last month is still
 * somewhere a person lives. The codes are CBSA codes in the same space as
 * `venues.metro`, which is why near-me matching compares codes instead of
 * typed city names.
 */
const HOME_METRO_WINDOW = 'all_time'

/** Sentinel for the "no home area" row: Radix Select forbids an empty value. */
const NO_METRO = '__none__'

/**
 * Resolve a stored CBSA code to something a person recognises.
 *
 * Falls back to the bare code rather than to a guess: a code with no matching
 * scene means the metro is not one we track shows in, and inventing a name for
 * it would hide that.
 */
export function useHomeMetroLabel(metro: string | null | undefined) {
  // Gated: with no area set there is no code to resolve, and the directory is
  // unbounded (every metro that has ever had five approved shows), so fetching
  // it to answer "null" is the one request most Library viewers would pay.
  const { data } = useChartScenes(HOME_METRO_WINDOW, Boolean(metro))
  return useMemo(() => {
    if (!metro) return null
    return data?.scenes.find(scene => scene.metro === metro)?.name ?? metro
  }, [data, metro])
}

interface HomeMetroSelectProps {
  /** The currently stored CBSA code, or null when no home area is set. */
  metro: string | null | undefined
  className?: string
  /** Accessible name — the visible label differs between the two call sites. */
  ariaLabel?: string
  onSaved?: () => void
}

/**
 * The one control that writes `user_preferences.home_metro` (PSY-1907).
 *
 * Shared by the Library alerts bar and the Settings "Your area" card because
 * both edit the same single value, and a second implementation is where the
 * clear-to-null path would get forgotten.
 */
export function HomeMetroSelect({
  metro,
  className,
  ariaLabel = 'Your area',
  onSaved,
}: HomeMetroSelectProps) {
  const { data, isLoading } = useChartScenes(HOME_METRO_WINDOW)
  const setHomeMetro = useSetHomeMetro()

  const scenes = data?.scenes ?? []

  // The offered list is NARROWER than what the server accepts: it is metros
  // with past shows above a floor, while the backend validates against the
  // whole CBSA dataset. So a legitimately stored area can be missing from it,
  // and without this row the Select would fall back to its placeholder and
  // show a SET area as unset, inviting the user to overwrite it by accident.
  const storedIsListed = !metro || scenes.some(scene => scene.metro === metro)

  // The list can also come back empty (a brand-new index, or an outage). Say
  // so rather than rendering a select whose only row is "no home area", which
  // reads as a broken control.
  if (!isLoading && scenes.length === 0 && storedIsListed) {
    return (
      <p className={cn('text-sm text-muted-foreground', className)}>
        No metros are available to choose from yet. An area can be set once the
        index tracks shows in one.
      </p>
    )
  }

  return (
    <div className={cn('flex flex-wrap items-center gap-2', className)}>
      <Select
        value={metro ?? NO_METRO}
        disabled={isLoading || setHomeMetro.isPending}
        onValueChange={value => {
          const next = value === NO_METRO ? null : value
          if (next === (metro ?? null)) return
          setHomeMetro.mutate(next, { onSuccess: () => onSaved?.() })
        }}
      >
        <SelectTrigger
          size="sm"
          className="w-[280px]"
          aria-label={ariaLabel}
        >
          <SelectValue placeholder="Choose your area" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NO_METRO}>No home area</SelectItem>
          {!storedIsListed && metro && (
            <SelectItem value={metro}>{metro}</SelectItem>
          )}
          {scenes.map(scene => (
            <SelectItem key={scene.metro} value={scene.metro}>
              {scene.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {setHomeMetro.isPending && (
        <Loader2
          className="h-4 w-4 animate-spin text-muted-foreground"
          aria-hidden
        />
      )}

      {setHomeMetro.isError && (
        <span className="text-xs text-destructive" role="alert">
          Couldn&apos;t save your area. Try again.
        </span>
      )}
    </div>
  )
}
