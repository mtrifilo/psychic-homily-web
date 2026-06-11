'use client'

import { useState } from 'react'
import Link from 'next/link'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { ProfileSectionAction } from './ProfileSectionAction'
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
// Collapsed row budget per the design board's notes density.
const COLLAPSED_COUNT = 5

export function ProfileFieldNotes({ username }: ProfileFieldNotesProps) {
  // Fetch the API max up front and slice client-side: the hook's query key
  // doesn't include limit, so a refetch-on-expand would be served from cache
  // and silently no-op. "View all →" reveals the fetched rows in place
  // (decision 2026-06-10: no dedicated per-user list routes yet).
  const [expanded, setExpanded] = useState(false)
  const { data, error } = useUserFieldNotes(username, { limit: 100 })

  if (error || !data || data.total === 0) return null

  return (
    <section aria-label="Field notes and reviews">
      <SectionHeader
        title="Field notes & reviews"
        as="h2"
        size="md"
        variant="title"
        action={
          !expanded && data.field_notes.length > COLLAPSED_COUNT ? (
            <ProfileSectionAction
              label="View all →"
              onClick={() => setExpanded(true)}
              ariaLabel={
                data.total > data.field_notes.length
                  ? `View the first ${data.field_notes.length} of ${data.total} field notes`
                  : `View all ${data.total} field notes`
              }
            />
          ) : undefined
        }
      />
      <div className="mt-1 divide-y divide-border/60">
        {(expanded ? data.field_notes : data.field_notes.slice(0, COLLAPSED_COUNT)).map(note => (
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
        {expanded && data.total > data.field_notes.length && (
          <p className="py-2 text-xs text-muted-foreground">
            + {data.total - data.field_notes.length} more
          </p>
        )}
      </div>
    </section>
  )
}
