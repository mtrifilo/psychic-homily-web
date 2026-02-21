'use client'

import { Loader2 } from 'lucide-react'
import { useSetFavoriteCities } from '@/lib/hooks/useFavoriteCities'

interface CityState {
  city: string
  state: string
}

interface SaveDefaultsButtonProps {
  selectedCities: CityState[]
  favoriteCities: CityState[]
}

export function SaveDefaultsButton({
  selectedCities,
  favoriteCities,
}: SaveDefaultsButtonProps) {
  const setFavoriteCities = useSetFavoriteCities()

  const handleSave = () => {
    setFavoriteCities.mutate(selectedCities)
  }

  const handleClear = () => {
    setFavoriteCities.mutate([])
  }

  if (setFavoriteCities.isPending) {
    return (
      <span className="text-xs text-muted-foreground flex items-center gap-1 self-center">
        <Loader2 className="h-3 w-3 animate-spin" />
        Saving...
      </span>
    )
  }

  // If the user selected cities, show "Save as default"
  // If the user cleared to "All" but has saved favorites, show "Clear defaults"
  if (selectedCities.length > 0) {
    return (
      <button
        onClick={handleSave}
        className="text-xs text-primary hover:text-primary/80 hover:underline underline-offset-2 transition-colors self-center whitespace-nowrap"
      >
        Save as default
      </button>
    )
  }

  if (favoriteCities.length > 0) {
    return (
      <button
        onClick={handleClear}
        className="text-xs text-muted-foreground hover:text-primary hover:underline underline-offset-2 transition-colors self-center whitespace-nowrap"
      >
        Clear defaults
      </button>
    )
  }

  return null
}
