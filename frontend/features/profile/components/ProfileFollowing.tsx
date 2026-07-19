'use client'

import Link from 'next/link'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { ProfileEmptyPrompt } from './ProfileEmptyPrompt'
import { ProfileSectionAction } from './ProfileSectionAction'
import { useUserFollowing } from '@/features/auth'
import type { FollowingEntity } from '@/features/auth'

interface ProfileFollowingProps {
  username: string
  /** Owner sees a "Manage" action linking to their follows page. */
  isOwner?: boolean
}

// Following lists artists/venues/labels/festivals. Tag-following does not
// exist in the backend (PSY-1045 dropped the mocked TAGS row), and shows use
// the save action rather than follow, so neither appears here.
const TYPE_ROWS: Array<{
  type: FollowingEntity['entity_type']
  label: string
  href: (slug: string) => string
}> = [
  { type: 'artist', label: 'Artists', href: slug => `/artists/${slug}` },
  { type: 'venue', label: 'Venues', href: slug => `/venues/${slug}` },
  { type: 'label', label: 'Labels', href: slug => `/labels/${slug}` },
  { type: 'festival', label: 'Festivals', href: slug => `/festivals/${slug}` },
  { type: 'scene', label: 'Scenes', href: slug => `/scenes/${slug}` },
]

/**
 * The "Following" section of the public profile (PSY-1045): the entities a
 * user follows, grouped by type as dense inline rows.
 *
 * Privacy shapes (server-gated by the `following` setting):
 * - visible → grouped rows
 * - count_only → total + empty list → single count line, no names
 * - hidden → 404 → section omitted entirely
 */
export function ProfileFollowing({
  username,
  isOwner = false,
}: ProfileFollowingProps) {
  // limit=100 (API max) — the showcase shows everything it can in one page;
  // per-type counts below are derived from this page, so >100 follows would
  // undercount (disclosed trade-off, no per-type totals on the endpoint).
  const { data, error } = useUserFollowing(username, { limit: 100 })

  // hidden (404) or any error → omit; loading → omit (section pops in).
  if (error || !data) return null
  // Visitors with zero follows: omit. Owners get an empty CTA (PSY-1489).
  if (data.total === 0 && !isOwner) return null

  const isEmpty = data.total === 0
  const isCountOnly = data.total > 0 && data.following.length === 0

  return (
    <section aria-label="Following">
      <SectionHeader
        title="Following"
        as="h2"
        size="md"
        variant="title"
        action={
          isOwner && !isEmpty ? (
            <ProfileSectionAction
              label="Manage"
              href="/library?tab=artists"
              ariaLabel="Manage who you follow"
            />
          ) : undefined
        }
      />
      {isEmpty ? (
        <ProfileEmptyPrompt
          message="Shape your taste graph and get show alerts — follow artists, venues & labels."
          ctaLabel="Browse"
          ctaHref="/artists"
        />
      ) : isCountOnly ? (
        <p className="text-sm text-muted-foreground mt-2">
          Follows{' '}
          <span className="text-foreground font-medium tabular-nums">
            {data.total}
          </span>{' '}
          {data.total === 1 ? 'artist, venue or label' : 'artists, venues & labels'}{' '}
          — lists hidden by this member.
        </p>
      ) : (
        <dl className="mt-2 space-y-1.5">
          {TYPE_ROWS.map(row => {
            const items = data.following.filter(
              f => f.entity_type === row.type
            )
            if (items.length === 0) return null
            return (
              <div key={row.type} className="flex items-baseline gap-3 text-sm">
                <dt className="w-20 shrink-0 text-[11px] uppercase tracking-wider text-muted-foreground">
                  {row.label}
                </dt>
                <dd className="min-w-0 flex-1 leading-relaxed">
                  {items.map((f, i) => (
                    <span key={`${f.entity_type}-${f.entity_id}`}>
                      {i > 0 && (
                        <span className="text-muted-foreground"> · </span>
                      )}
                      {f.slug ? (
                        <Link
                          href={row.href(f.slug)}
                          className="hover:text-primary hover:underline"
                        >
                          {f.name}
                        </Link>
                      ) : (
                        f.name
                      )}
                    </span>
                  ))}
                </dd>
                <dd className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {items.length}
                </dd>
              </div>
            )
          })}
        </dl>
      )}
    </section>
  )
}
