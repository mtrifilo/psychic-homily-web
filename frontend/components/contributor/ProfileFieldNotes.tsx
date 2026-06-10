'use client'

import Link from 'next/link'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { useUserFieldNotes } from '@/features/auth'

interface ProfileFieldNotesProps {
  username: string
}

/**
 * Field notes & reviews on the public profile (PSY-1045): the visible field
 * notes the user has written, newest first, titled by show. No granular
 * privacy gate (notes are already public on each show page); only the master
 * profile-visibility gate applies server-side. No star rating by design
 * (2026-06-09 decision).
 */
export function ProfileFieldNotes({ username }: ProfileFieldNotesProps) {
  const { data, error } = useUserFieldNotes(username, { limit: 5 })

  if (error || !data || data.total === 0) return null

  return (
    <section aria-label="Field notes and reviews">
      <SectionHeader title="Field notes & reviews" as="h2" size="md" />
      <div className="mt-1 divide-y divide-border/60">
        {data.field_notes.map(note => (
          <div key={note.id} className="py-2.5">
            <p className="text-sm font-medium">
              {note.show_slug ? (
                <Link
                  href={`/shows/${note.show_slug}#field-notes`}
                  className="hover:text-primary hover:underline"
                >
                  {note.show_title || 'Untitled show'}
                </Link>
              ) : (
                note.show_title || 'Untitled show'
              )}
            </p>
            {/* body is plain text from the author; render the raw text
                truncated rather than the HTML to keep rows dense. */}
            <p className="mt-0.5 line-clamp-2 text-sm text-muted-foreground">
              &ldquo;{note.body}&rdquo;
            </p>
          </div>
        ))}
        {data.total > data.field_notes.length && (
          <p className="py-2 text-xs text-muted-foreground">
            + {data.total - data.field_notes.length} more
          </p>
        )}
      </div>
    </section>
  )
}
