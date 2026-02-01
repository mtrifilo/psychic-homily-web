'use client'

import { Star } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useFavoriteVenueToggle } from '@/lib/hooks/useFavoriteVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useState } from 'react'

interface FavoriteVenueButtonProps {
  venueId: number
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'sm' | 'md' | 'lg'
  showLabel?: boolean
}

export function FavoriteVenueButton({
  venueId,
  variant = 'ghost',
  size = 'sm',
  showLabel = false,
}: FavoriteVenueButtonProps) {
  const { isAuthenticated } = useAuthContext()
  const { isFavorited, isLoading, toggle, error } = useFavoriteVenueToggle(venueId)
  const [showError, setShowError] = useState(false)

  // Don't render if not authenticated
  if (!isAuthenticated) {
    return null
  }

  const handleClick = async (e: React.MouseEvent) => {
    e.preventDefault() // Prevent any parent link clicks
    e.stopPropagation()

    try {
      setShowError(false)
      await toggle()
    } catch (err) {
      setShowError(true)
      // Auto-hide error after 3 seconds
      setTimeout(() => setShowError(false), 3000)
    }
  }

  const iconSize = size === 'sm' ? 'h-4 w-4' : size === 'md' ? 'h-5 w-5' : 'h-6 w-6'
  const buttonSize = size === 'sm' ? 'h-8 w-8' : size === 'md' ? 'h-10 w-10' : 'h-12 w-12'

  return (
    <div className="relative">
      <Button
        variant={variant}
        size="icon"
        onClick={handleClick}
        disabled={isLoading}
        className={`${buttonSize} p-0 ${showLabel ? 'w-auto px-3 gap-2' : ''}`}
        title={isFavorited ? 'Remove from Favorites' : 'Add to Favorites'}
        aria-label={isFavorited ? 'Remove from Favorites' : 'Add to Favorites'}
      >
        <Star
          className={`${iconSize} transition-all ${
            isFavorited
              ? 'fill-amber-500 text-amber-500'
              : 'text-muted-foreground hover:text-foreground'
          } ${isLoading ? 'opacity-50' : ''}`}
        />
        {showLabel && (
          <span className="text-sm">
            {isFavorited ? 'Favorited' : 'Favorite'}
          </span>
        )}
      </Button>

      {/* Error tooltip */}
      {showError && error && (
        <div className="absolute top-full left-1/2 -translate-x-1/2 mt-2 px-3 py-1.5 bg-destructive text-destructive-foreground text-xs rounded-md whitespace-nowrap z-50 shadow-lg">
          Failed to {isFavorited ? 'remove' : 'add'} favorite
        </div>
      )}
    </div>
  )
}
