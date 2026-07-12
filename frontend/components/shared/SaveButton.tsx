'use client'

import { Heart } from 'lucide-react'
import { usePathname, useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { useSaveShowToggle, useShowSaveCount } from '@/features/shows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useState } from 'react'
import { cn } from '@/lib/utils'

interface SaveButtonProps {
  showId: number
  variant?: 'default' | 'ghost' | 'outline' | 'bracket'
  size?: 'sm' | 'md' | 'lg'
  showLabel?: boolean
  /**
   * Pre-fetched data from the batch save-count endpoint, avoids an extra
   * request. Bundled (rather than two loose props) so a caller cannot supply
   * the count without the viewer's own saved state — mirrors FollowButton.
   */
  saveData?: { save_count: number; is_saved: boolean }
  className?: string
  disabled?: boolean
}

export function SaveButton({
  showId,
  variant = 'ghost',
  size = 'sm',
  showLabel = false,
  saveData,
  className,
  disabled = false,
}: SaveButtonProps) {
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()
  const pathname = usePathname()

  // List views pass saveData in from one batched request. Standalone usages
  // (show detail page, library rows) fetch their own.
  const { data: single } = useShowSaveCount(showId, isAuthenticated, !saveData)
  const data = saveData ?? single

  const isSaved = data?.is_saved ?? false
  const saveCount = data?.save_count ?? 0

  const { isLoading, toggle, error } = useSaveShowToggle(showId, isSaved)
  const isDisabled = disabled || isLoading
  const [showError, setShowError] = useState(false)

  const handleClick = async (e: React.MouseEvent) => {
    e.preventDefault() // Prevent any parent link clicks
    e.stopPropagation()

    // Matches FollowButton: render for anonymous visitors so the public save
    // count stays visible, and send them to sign-in on click.
    if (!isAuthenticated) {
      router.push(`/auth?returnTo=${encodeURIComponent(pathname)}`)
      return
    }
    if (isDisabled) return

    try {
      setShowError(false)
      await toggle()
    } catch {
      setShowError(true)
      // Auto-hide error after 3 seconds
      setTimeout(() => setShowError(false), 3000)
    }
  }

  const label = !isAuthenticated
    ? 'Sign in to save'
    : isSaved
      ? 'Remove from My List'
      : 'Add to My List'

  if (variant === 'bracket') {
    return (
      <div className="relative inline-flex">
        <BracketLink
          label={isSaved ? 'Saved' : 'Save'}
          active={isSaved}
          onClick={handleClick}
          disabled={isDisabled}
          className={cn('font-mono text-[11px]', className)}
          ariaLabel={label}
        />
        {showError && error ? (
          <div className="absolute left-1/2 top-full z-50 mt-2 -translate-x-1/2 whitespace-nowrap rounded-sm bg-destructive px-3 py-1.5 text-xs text-destructive-foreground shadow-sm">
            Failed to {isSaved ? 'remove' : 'save'} show
          </div>
        ) : null}
      </div>
    )
  }

  const iconSize =
    size === 'sm' ? 'h-4 w-4' : size === 'md' ? 'h-5 w-5' : 'h-6 w-6'
  const buttonSize =
    size === 'sm' ? 'h-8 w-8' : size === 'md' ? 'h-10 w-10' : 'h-12 w-12'
  const hasCount = saveCount > 0

  return (
    <div className="relative">
      <Button
        variant={variant}
        size="icon"
        onClick={handleClick}
        disabled={isDisabled}
        className={cn(
          buttonSize,
          'p-0',
          (showLabel || hasCount) && 'w-auto px-3 gap-1.5',
          className
        )}
        title={label}
        aria-label={hasCount ? `${label} (${saveCount} saved)` : label}
      >
        <Heart
          className={`${iconSize} transition-all ${
            isSaved
              ? 'fill-red-500 text-red-500'
              : 'text-muted-foreground hover:text-foreground'
          } ${isLoading ? 'opacity-50' : ''}`}
        />
        {hasCount && (
          <span className="text-xs tabular-nums text-muted-foreground">
            {saveCount}
          </span>
        )}
        {showLabel && (
          <span className="text-sm">{isSaved ? 'Saved' : 'Save'}</span>
        )}
      </Button>

      {/* Error tooltip */}
      {showError && error && (
        <div className="absolute top-full left-1/2 -translate-x-1/2 mt-2 px-3 py-1.5 bg-destructive text-destructive-foreground text-xs rounded-md whitespace-nowrap z-50 shadow-sm">
          Failed to {isSaved ? 'remove' : 'save'} show
        </div>
      )}
    </div>
  )
}
