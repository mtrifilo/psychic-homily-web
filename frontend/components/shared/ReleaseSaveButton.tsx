'use client'

import { Bookmark, Loader2 } from 'lucide-react'
import { usePathname, useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useReleaseSaveCount, useReleaseSaveToggle } from '@/features/releases'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { useAutoDismissBanner } from '@/lib/hooks/common'
import {
  resolveBatchedSaveData,
  type BatchedSaveData,
} from './batchedSaveData'

// How long a save failure stays on screen before auto-hiding.
const ERROR_DISMISS_MS = 3000

interface ReleaseSaveButtonProps {
  releaseId: number
  /**
   * Pre-fetched data from the batch save-count endpoint. List surfaces must pass
   * `'pending'` while their batch is in flight (build it with `batchedSaveFor`) —
   * see BatchedSaveData for why omitting it made every card race the batch.
   */
  saveData?: BatchedSaveData
  variant?: 'button' | 'bracket' | 'text'
  className?: string
  disabled?: boolean
  actionLabel?: string
  actionAriaLabel?: string
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
  actionLabel,
  actionAriaLabel,
}: ReleaseSaveButtonProps) {
  const { isAuthenticated, authStatus, user } = useAuthContext()
  const router = useRouter()
  const pathname = usePathname()
  // While a batch owns this release the prop is 'pending', which suppresses the
  // per-item request rather than racing the batch that replaces it.
  const { value: batched, shouldSelfFetch } = resolveBatchedSaveData(saveData)
  // The save COUNT is public, so an anonymous viewer still fetches — unlike the
  // bracket FollowButton, this control paints the count it gets back.
  //
  // `authStatus !== 'pending'` is the write-side half of that: the cache key
  // carries `isAuthenticated`, which is false during the unsettled window, and
  // a request issued there carries the viewer's cookie. Without the guard a
  // signed-in viewer's `is_saved` lands under the viewer-less key and is
  // painted back to them once their session ends within the same SPA session.
  const { data: fetched, isLoading: statusLoading } = useReleaseSaveCount(
    releaseId,
    isAuthenticated,
    shouldSelfFetch && authStatus !== 'pending',
    user?.id
  )
  const data = batched ?? fetched
  const isSaved = data?.is_saved ?? false
  const saveCount = data?.save_count ?? 0
  const { toggle, isLoading, error } = useReleaseSaveToggle(
    releaseId,
    isSaved,
    user?.id
  )
  // Shared auto-dismiss primitive rather than a hand-rolled timer, which must
  // not outlive unmount. See useAutoDismissBanner / useDismissTimer (PSY-1664).
  const {
    value: showError,
    show: showSaveError,
    clear: clearSaveError,
  } = useAutoDismissBanner<true>(ERROR_DISMISS_MS)
  // `authStatus === 'pending'` for the reason FollowButton and SaveButton carry
  // it: all three variants ship ENABLED in server HTML and opt into
  // pre-hydration click replay, and the handler routes on `!isAuthenticated`,
  // which reads false both for a viewer with no session and for one whose
  // profile has not arrived. A replayed click in that window sends a signed-in
  // viewer to /auth.
  const isDisabled =
    disabled || statusLoading || isLoading || authStatus === 'pending'

  const handleClick = async (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    // Unreachable while `isDisabled` includes 'pending' (React suppresses
    // onClick on a disabled control); kept because the redirect below cannot
    // distinguish "no session" from "profile in flight", and it sits ahead of
    // the `isDisabled` bail, which runs after the redirect.
    if (authStatus === 'pending') return
    if (!isAuthenticated) {
      const returnTo = `${pathname}${window.location.search}`
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }
    if (isDisabled) return
    try {
      clearSaveError()
      await toggle()
    } catch {
      showSaveError(true)
    }
  }

  const label = isSaved ? 'Saved' : 'Save'
  const ariaLabel =
    actionAriaLabel ?? (isSaved ? 'Remove saved release' : 'Save release')
  return (
    <div className="relative inline-flex">
      {variant === 'bracket' ? (
        <BracketLink
          label={actionLabel ?? label}
          active={isSaved}
          onClick={handleClick}
          disabled={isDisabled}
          className={cn('font-mono text-[11px]', className)}
          ariaLabel={ariaLabel}
        />
      ) : variant === 'text' ? (
        <button
          {...replayOnHydrate}
          type="button"
          onClick={handleClick}
          disabled={isDisabled}
          className={cn(
            'whitespace-nowrap font-mono text-[11px] text-muted-foreground transition-colors hover:text-destructive disabled:cursor-wait disabled:opacity-60',
            className
          )}
          aria-label={ariaLabel}
        >
          {isLoading ? 'removing…' : (actionLabel ?? label)}
        </button>
      ) : (
        <Button
          {...replayOnHydrate}
          type="button"
          variant={isSaved ? 'secondary' : 'ghost'}
          size="sm"
          onClick={handleClick}
          disabled={isDisabled}
          className={cn('h-8 gap-1.5', className)}
          aria-label={`${ariaLabel}${saveCount > 0 ? ` (${saveCount} saved)` : ''}`}
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
