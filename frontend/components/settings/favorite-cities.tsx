'use client'

import { useMemo } from 'react'
import { MapPin, Loader2, CheckCircle2, AlertCircle } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useProfile } from '@/lib/hooks/useAuth'
import { useShowCities } from '@/lib/hooks/useShows'
import { useSetFavoriteCities } from '@/lib/hooks/useFavoriteCities'
import { FilterChip } from '@/components/filters/FilterChip'

interface CityState {
  city: string
  state: string
}

function cityKey(c: CityState): string {
  return `${c.city}|${c.state}`
}

export function FavoriteCitiesSettings() {
  const { data: profileData } = useProfile()
  const setFavoriteCities = useSetFavoriteCities()

  const timezone =
    typeof window !== 'undefined'
      ? Intl.DateTimeFormat().resolvedOptions().timeZone
      : 'America/Phoenix'

  const { data: citiesData, isLoading: citiesLoading } = useShowCities({ timezone })

  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  const favoriteSet = useMemo(
    () => new Set(favoriteCities.map(cityKey)),
    [favoriteCities]
  )

  const availableCities = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
      })) ?? [],
    [citiesData?.cities]
  )

  const handleToggle = (city: string, state: string) => {
    const key = cityKey({ city, state })
    let newCities: CityState[]
    if (favoriteSet.has(key)) {
      newCities = favoriteCities.filter(c => cityKey(c) !== key)
    } else {
      newCities = [...favoriteCities, { city, state }]
    }
    setFavoriteCities.mutate(newCities)
  }

  const handleClearAll = () => {
    setFavoriteCities.mutate([])
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <MapPin className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-lg">Favorite Cities</CardTitle>
        </div>
        <CardDescription>
          Choose your default cities for the show calendar. Selected cities will be pre-filtered when you visit the home page or shows page.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {citiesLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading cities...
            </div>
          ) : availableCities.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No cities with upcoming shows found.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                {availableCities.map(city => {
                  const isActive = favoriteSet.has(cityKey(city))
                  return (
                    <FilterChip
                      key={`${city.city}-${city.state}`}
                      label={`${city.city}, ${city.state}`}
                      isActive={isActive}
                      onClick={() => handleToggle(city.city, city.state)}
                      count={city.count}
                    />
                  )
                })}
              </div>

              {favoriteCities.length > 0 && (
                <div className="flex items-center justify-between pt-2">
                  <p className="text-xs text-muted-foreground">
                    {favoriteCities.length} {favoriteCities.length === 1 ? 'city' : 'cities'} selected
                  </p>
                  <button
                    onClick={handleClearAll}
                    disabled={setFavoriteCities.isPending}
                    className="text-xs text-muted-foreground hover:text-primary hover:underline underline-offset-2 transition-colors"
                  >
                    Clear all
                  </button>
                </div>
              )}

              {setFavoriteCities.isPending && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Saving...
                </div>
              )}

              {setFavoriteCities.isSuccess && !setFavoriteCities.isPending && (
                <div className="flex items-center gap-2 text-xs text-emerald-600 dark:text-emerald-400">
                  <CheckCircle2 className="h-3 w-3" />
                  Saved
                </div>
              )}

              {setFavoriteCities.isError && (
                <div className="flex items-center gap-2 text-xs text-destructive">
                  <AlertCircle className="h-3 w-3" />
                  Failed to save. Please try again.
                </div>
              )}
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
