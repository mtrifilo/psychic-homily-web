'use client'

import { useState } from 'react'
import Link from 'next/link'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { statNumberFormatter } from '@/components/shared/StatsList'
import { useUserPublicCollections } from '@/features/collections'
import { ProfileEmptyPrompt } from './ProfileEmptyPrompt'
import { ProfileSectionAction } from './ProfileSectionAction'

interface ProfileCollectionsProps {
  username: string
  isOwner: boolean
}

// Collapsed row budget per the design board's collections density.
const COLLAPSED_COUNT = 5

/**
 * The Collections section of the public profile (PSY-1062): dense one-line
 * rows per the redesign board — title left, "N items · M likes" in mono right
 * — replacing the thumbnail cards. "View all →" expands in place (decision
 * 2026-06-10: no dedicated per-user list routes yet).
 */
export function ProfileCollections({
  username,
  isOwner,
}: ProfileCollectionsProps) {
  const [expanded, setExpanded] = useState(false)
  const { data } = useUserPublicCollections(username)

  const collections = data?.collections ?? []
  const total = data?.total ?? 0

  // Loading (data undefined) → omit; the section pops in like its siblings.
  if (!data) return null
  if (total === 0 && !isOwner) return null

  const visible = expanded
    ? collections
    : collections.slice(0, COLLAPSED_COUNT)
  const hasMore = !expanded && collections.length > COLLAPSED_COUNT

  return (
    <section aria-label="Collections">
      <SectionHeader
        title="Collections"
        as="h2"
        size="md"
        variant="title"
        action={
          hasMore ? (
            <ProfileSectionAction
              label="View all →"
              onClick={() => setExpanded(true)}
              ariaLabel={
                total > collections.length
                  ? `View the first ${collections.length} of ${total} collections`
                  : `View all ${total} collections`
              }
            />
          ) : undefined
        }
      />
      {total === 0 ? (
        <ProfileEmptyPrompt
          message="Curate a list worth sharing — your public collections show up here."
          ctaLabel="Start a collection"
          ctaHref="/collections"
        />
      ) : (
        <div className="mt-1 divide-y divide-border/60">
          {visible.map(collection => (
            <div
              key={collection.id}
              className="flex items-baseline justify-between gap-4 py-2 text-sm"
            >
              <Link
                href={`/collections/${collection.slug}`}
                className="min-w-0 flex-1 truncate font-medium hover:text-primary hover:underline"
              >
                {collection.title}
              </Link>
              <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
                {statNumberFormatter.format(collection.item_count)}{' '}
                {collection.item_count === 1 ? 'item' : 'items'} ·{' '}
                {statNumberFormatter.format(collection.like_count)}{' '}
                {collection.like_count === 1 ? 'like' : 'likes'}
              </span>
            </div>
          ))}
          {expanded && total > collections.length && (
            <p className="py-2 text-xs text-muted-foreground">
              + {total - collections.length} more
            </p>
          )}
        </div>
      )}
    </section>
  )
}
