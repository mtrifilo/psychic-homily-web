'use client'

import { useState } from 'react'
import { Bookmark, Loader2 } from 'lucide-react'
import { usePathname, useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useReleaseSaveCount, useReleaseSaveToggle } from '@/features/releases'
import { cn } from '@/lib/utils'

interface ReleaseSaveButtonProps {
  releaseId: number
  saveData?: { save_count: number; is_saved: boolean }
  variant?: 'button' | 'bracket'
  className?: string
  disabled?: boolean
  bracketLabel?: string
  ariaLabel?: string
}

/**
 * First-class Save/Saved control for releases. The backend preserves legacy
 * `bookmark` rows internally, but this component intentionally exposes the
 * same user vocabulary and auth-gate behavior as show saving.
 */
export function ReleaseSaveButton({
  releaseId,
  saveData,
  variant = 'button',
  className,
  disabled = false,
  bracketLabel,
  ariaLabel,
}: ReleaseSaveButtonProps) {
  const { isAuthenticated, user } = useAuthContext()
  const router = useRouter()
  const pathname = usePathname()
  const { data: fetched, isLoading: statusLoading } = useReleaseSaveCount(
    releaseId,
    isAuthenticated,
    !saveData,
    user?.id
  )
  const data = saveData ?? fetched
  const isSaved = data?.is_saved ?? false
  const saveCount = data?.save_count ?? 0
  const { toggle, isLoading, error } = useReleaseSaveToggle(
    releaseId,
    isSaved,
    user?.id
  )
  const [showError, setShowError] = useState(false)
  const isDisabled = disabled || statusLoading || isLoading

  const handleClick = async (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    if (!isAuthenticated) {
      const returnTo = `${pathname}${window.location.search}`
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }
    if (isDisabled) return
    try {
      setShowError(false)
      await toggle()
    } catch {
      setShowError(true)
      setTimeout(() => setShowError(false), 3000)
    }
  }

  const label = isSaved ? 'Saved' : 'Save'
  return (
    <div className="relative inline-flex">
      {variant === 'bracket' ? (
        <BracketLink
          label={bracketLabel ?? label}
          active={isSaved}
          onClick={handleClick}
          disabled={isDisabled}
          className={cn('font-mono text-[11px]', className)}
          ariaLabel={
            ariaLabel ?? (isSaved ? 'Remove saved release' : 'Save release')
          }
        />
      ) : (
        <Button
          type="button"
          variant={isSaved ? 'secondary' : 'ghost'}
          size="sm"
          onClick={handleClick}
          disabled={isDisabled}
          className={cn('h-8 gap-1.5', className)}
          aria-label={`${isSaved ? 'Remove saved release' : 'Save release'}${saveCount > 0 ? ` (${saveCount} saved)` : ''}`}
        >
          {isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Bookmark className={cn('h-4 w-4', isSaved && 'fill-current')} />
          )}
          <span>{label}</span>
          {saveCount > 0 ? (
            <span className="font-mono text-xs text-muted-foreground">
              {saveCount}
            </span>
          ) : null}
        </Button>
      )}
      {showError && error ? (
        <div className="absolute left-1/2 top-full z-50 mt-2 -translate-x-1/2 whitespace-nowrap rounded-sm bg-destructive px-3 py-1.5 text-xs text-destructive-foreground shadow-sm">
          Failed to {isSaved ? 'remove' : 'save'} release
        </div>
      ) : null}
    </div>
  )
}
