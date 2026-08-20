'use client'

import { useState } from 'react'
import { ClipboardList } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
// Imported from the module rather than the feature barrel: the barrel also
// re-exports PasskeyRegisterButton, which drags @simplewebauthn/browser onto
// every page that renders this section.
import { SignInPrompt } from '@/features/auth/components/SignInPrompt'
import { StatusBanner } from '@/components/shared'
import { hasShowStarted } from '@/lib/utils/showTiming'
import {
  useFieldNotes,
  useCreateFieldNote,
  formatCommentSubmissionError,
} from '../hooks'
import { FIELD_NOTES_SECTION_ANCHOR } from '../anchors'
import { FieldNoteForm } from './FieldNoteForm'
import { FieldNoteCard } from './FieldNoteCard'
import type { Comment, CreateFieldNoteInput } from '../types'

interface ShowArtist {
  id: number
  name: string
}

interface FieldNotesSectionProps {
  showId: number
  showDate: string
  artists?: ShowArtist[]
}

export function FieldNotesSection({ showId, showDate, artists = [] }: FieldNotesSectionProps) {
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading } = useFieldNotes(showId)
  const createMutation = useCreateFieldNote()
  // PSY-513: track the author's pending-review field note so we can render
  // it optimistically alongside the public list (which filters out
  // pending_review). De-duped once the canonical row appears post-approval.
  const [pendingNote, setPendingNote] = useState<Comment | null>(null)
  // PSY-608: bumped on every successful submit so FieldNoteForm clears its
  // local state. The form keeps the draft on error so the user can retry
  // without retyping (mirrors CommentForm's resetSignal pattern).
  const [submitGeneration, setSubmitGeneration] = useState(0)

  const fieldNotes = data?.comments ?? []
  const total = data?.total ?? 0
  // The START INSTANT: the API rejects a note on a show whose `event_date` is
  // still in the future (`ErrFieldNoteShowFuture`), so this gate's job is to
  // agree with that boundary exactly. A stricter one here would hide a form the
  // API would have accepted; a looser one would offer a form it will 400.
  //
  // `showTiming` also exports `isShowPast`, the venue-local calendar-day
  // boundary. It is the wrong one here and its only consumer is the share
  // card's cache window, not any reader-facing surface.
  const isFuture = !hasShowStarted(showDate)

  const hasCanonicalPending =
    pendingNote !== null && fieldNotes.some((c) => c.id === pendingNote.id)
  const effectivePending = hasCanonicalPending ? null : pendingNote

  const handleCreate = (input: CreateFieldNoteInput) => {
    createMutation.mutate(
      { showId, input },
      {
        onSuccess: (created) => {
          if (created.visibility === 'pending_review') {
            setPendingNote(created)
          }
          // PSY-608: clear the form ONLY on success. On 4xx the form
          // retains the draft so the user can retry.
          setSubmitGeneration((g) => g + 1)
        },
      }
    )
  }

  return (
    <section
      id={FIELD_NOTES_SECTION_ANCHOR}
      className="mt-8 scroll-mt-20"
      data-testid="field-notes-section"
    >
      {/* Header */}
      <div className="flex items-center gap-2 mb-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <ClipboardList className="h-5 w-5" />
          Field Notes
          {total > 0 && (
            <span className="text-sm font-normal text-muted-foreground">
              ({total})
            </span>
          )}
        </h2>
      </div>

      {/* Future show gate */}
      {isFuture ? (
        <p
          className="text-sm text-muted-foreground py-4"
          data-testid="future-show-message"
        >
          {/* No date. The gate opens at the start instant, so any date or time
              in this sentence is a promise about a boundary this component
              cannot keep: `hasShowStarted` is evaluated once per render and
              nothing re-renders when doors open, so a reader sitting on the
              page at 7:55 for an 8:00 show would watch a stated time pass with
              the form still shut. The date and start time are already on the
              page header, in the venue's zone, a section above.

              This also drops a `toLocaleDateString` with no `timeZone`, which
              formatted against the SERVER's clock during SSR and the reader's
              on hydration. */}
          Field notes will be available after the show starts.
        </p>
      ) : (
        <>
          {/* Field note form */}
          {isAuthenticated ? (
            <div className="mb-6">
              <FieldNoteForm
                onSubmit={handleCreate}
                artists={artists}
                isPending={createMutation.isPending}
                errorMessage={formatCommentSubmissionError(
                  createMutation.error
                )}
                resetSignal={submitGeneration}
              />
            </div>
          ) : (
            <SignInPrompt
              testId="field-note-auth-gate"
              returnToHash={FIELD_NOTES_SECTION_ANCHOR}
            >
              to share your experience.
            </SignInPrompt>
          )}

          {/* PSY-513 / PSY-575: pending-review confirmation banner via the
              shared `StatusBanner` primitive. Only the author sees this. */}
          {effectivePending && (
            <StatusBanner
              variant="pending"
              testId="pending-review-banner"
              className="mb-4"
            >
              <p className="text-sm text-pending-foreground">
                Field note submitted — awaiting moderation. You&apos;ll see it here once an admin approves it.
              </p>
            </StatusBanner>
          )}

          {/* Field notes list */}
          {isLoading ? (
            <div className="space-y-4">
              {[1, 2, 3].map((i) => (
                <div key={i} className="animate-pulse space-y-2 rounded-lg border border-border/50 p-4">
                  <div className="h-3 w-32 bg-muted rounded" />
                  <div className="h-4 w-full bg-muted rounded" />
                  <div className="h-4 w-3/4 bg-muted rounded" />
                </div>
              ))}
            </div>
          ) : fieldNotes.length === 0 && !effectivePending ? (
            <p
              className="text-sm text-muted-foreground py-8 text-center"
              data-testid="field-notes-empty"
            >
              No field notes yet. Attend this show and share your experience!
            </p>
          ) : (
            <div className="space-y-4">
              {effectivePending && (
                <FieldNoteCard
                  comment={effectivePending}
                  showId={showId}
                  artists={artists}
                />
              )}
              {fieldNotes.map((note) => (
                <FieldNoteCard
                  key={note.id}
                  comment={note}
                  showId={showId}
                  artists={artists}
                />
              ))}
            </div>
          )}

          {/* Load more */}
          {data?.has_more && (
            <div className="mt-4 text-center">
              <button className="text-sm text-primary hover:underline">
                Load more field notes
              </button>
            </div>
          )}
        </>
      )}
    </section>
  )
}
