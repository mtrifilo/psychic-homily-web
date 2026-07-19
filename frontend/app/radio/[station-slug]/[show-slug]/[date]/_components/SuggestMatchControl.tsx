'use client'

import { useState } from 'react'
import { usePathname } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { InlineErrorBanner } from '@/components/shared'
import { ArtistSearch } from '@/features/artists'
import type { Artist } from '@/features/artists'
import { LoginPromptDialog } from '@/features/auth'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useCreatePlayMatchSuggestion,
  useOwnPlayMatchSuggestion,
} from '@/features/radio/hooks/usePlayMatchSuggestions'

const CTA_CLASS =
  'font-mono text-[10px] text-muted-foreground hover:text-foreground underline-offset-2 hover:underline disabled:opacity-50'

interface SuggestMatchControlProps {
  playId: number
  playArtistName: string
}

/**
 * Dense quiet CTA for unmatched playlist rows. Authenticated users open an
 * artist picker (existing artists only) + optional note; guests get a visible
 * sign-in prompt. Success shows "suggestion pending" without claiming a match.
 */
export function SuggestMatchControl({
  playId,
  playArtistName,
}: SuggestMatchControlProps) {
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()
  const { data: pending, isLoading: pendingLoading } = useOwnPlayMatchSuggestion(
    playId,
    isAuthenticated
  )
  const createSuggestion = useCreatePlayMatchSuggestion()

  const [pickerOpen, setPickerOpen] = useState(false)
  const [loginOpen, setLoginOpen] = useState(false)
  const [selected, setSelected] = useState<Artist | null>(null)
  const [note, setNote] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)

  if (isAuthenticated && pending) {
    return (
      <span
        className="font-mono text-[10px] text-muted-foreground"
        data-testid="suggest-match-pending"
      >
        suggestion pending
      </span>
    )
  }

  // While the mine query is in flight, keep a quiet placeholder so we don't
  // flash "suggestion pending" for rows that have no suggestion yet.
  if (isAuthenticated && pendingLoading) {
    return (
      <span
        className="font-mono text-[10px] text-muted-foreground/50"
        data-testid="suggest-match-loading"
        aria-hidden="true"
      >
        …
      </span>
    )
  }

  const resetPicker = () => {
    setSelected(null)
    setNote('')
    setSubmitError(null)
  }

  const handleCtaClick = () => {
    if (isAuthenticated) {
      resetPicker()
      setPickerOpen(true)
    } else {
      setLoginOpen(true)
    }
  }

  const handleOpenChange = (open: boolean) => {
    setPickerOpen(open)
    if (!open) resetPicker()
  }

  const handleSubmit = () => {
    if (!selected) return
    setSubmitError(null)
    createSuggestion.mutate(
      {
        playId,
        artistId: selected.id,
        note: note.trim() || undefined,
      },
      {
        onSuccess: () => {
          setPickerOpen(false)
          resetPicker()
        },
        onError: (err) => {
          setSubmitError(
            err instanceof Error ? err.message : 'Failed to submit suggestion'
          )
        },
      }
    )
  }

  return (
    <>
      <button
        type="button"
        className={CTA_CLASS}
        onClick={handleCtaClick}
        data-testid="suggest-match-cta"
      >
        [suggest a match]
      </button>

      {isAuthenticated && (
        <Dialog open={pickerOpen} onOpenChange={handleOpenChange}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Suggest a match</DialogTitle>
              <DialogDescription>
                Pick an existing artist for{' '}
                <span className="font-medium text-foreground">
                  {playArtistName}
                </span>
                . An admin will review before the play is linked.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-3">
              <ArtistSearch
                placeholder="Search artists…"
                onSelect={(artist) => {
                  setSelected(artist)
                  setSubmitError(null)
                }}
                className="max-w-none"
              />

              {selected && (
                <p
                  className="text-sm text-muted-foreground"
                  data-testid="suggest-match-selection"
                >
                  Suggesting:{' '}
                  <span className="font-medium text-foreground">
                    {selected.name}
                  </span>
                </p>
              )}

              <Textarea
                value={note}
                onChange={(e) => setNote(e.target.value.slice(0, 500))}
                placeholder="Optional note (why this match?)"
                rows={2}
                disabled={createSuggestion.isPending}
                className="resize-none text-sm"
                data-testid="suggest-match-note"
              />

              {submitError && (
                <InlineErrorBanner testId="suggest-match-error">
                  {submitError}
                </InlineErrorBanner>
              )}
            </div>

            <DialogFooter>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleOpenChange(false)}
                disabled={createSuggestion.isPending}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={handleSubmit}
                disabled={!selected || createSuggestion.isPending}
                data-testid="suggest-match-confirm"
              >
                {createSuggestion.isPending && (
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                )}
                Submit suggestion
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {!isAuthenticated && (
        <LoginPromptDialog
          open={loginOpen}
          onOpenChange={setLoginOpen}
          title="Sign in to suggest a match"
          description="You need to be signed in to suggest an artist match for unmatched playlist tracks."
          returnTo={pathname}
        />
      )}
    </>
  )
}
