'use client'

import { ClipboardList } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useFieldNotes, useCreateFieldNote } from '../hooks'
import { FieldNoteForm } from './FieldNoteForm'
import { FieldNoteCard } from './FieldNoteCard'
import type { CreateFieldNoteInput } from '../types'

interface ShowArtist {
  id: number
  name: string
}

interface FieldNotesSectionProps {
  showId: number
  showDate: string
  artists?: ShowArtist[]
}

function isShowInFuture(showDate: string): boolean {
  const eventDate = new Date(showDate)
  return eventDate > new Date()
}

function formatFutureDate(showDate: string): string {
  const date = new Date(showDate)
  return date.toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

export function FieldNotesSection({ showId, showDate, artists = [] }: FieldNotesSectionProps) {
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading } = useFieldNotes(showId)
  const createMutation = useCreateFieldNote()

  const fieldNotes = data?.comments ?? []
  const total = data?.total ?? 0
  const isFuture = isShowInFuture(showDate)

  const handleCreate = (input: CreateFieldNoteInput) => {
    createMutation.mutate({ showId, input })
  }

  return (
    <section className="mt-8" data-testid="field-notes-section">
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
          Field notes will be available after {formatFutureDate(showDate)}.
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
              />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground mb-6" data-testid="field-note-auth-gate">
              <a href="/login" className="text-primary hover:underline">
                Sign in
              </a>{' '}
              to share your experience.
            </p>
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
          ) : fieldNotes.length === 0 ? (
            <p
              className="text-sm text-muted-foreground py-8 text-center"
              data-testid="field-notes-empty"
            >
              No field notes yet. Attend this show and share your experience!
            </p>
          ) : (
            <div className="space-y-4">
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
