'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { ChevronRight } from 'lucide-react'
import { ALERTS_AREA_HREF } from '@/components/shared/followAlertChoices'
import { useHomeMetroState } from '@/features/auth/hooks/useAlertPreferences'
import { SHOW_ROW_GRID } from '@/features/shows/components/ShowRow'
import { useMarkNotificationsRead } from '@/features/notifications/hooks'
import { useLibraryFollowingCounts } from '@/lib/hooks/common/useFollow'
import { useAuthContext } from '@/lib/context/AuthContext'
import { cn } from '@/lib/utils'
import { useLibraryUpdates, type LibraryUpdate } from '../hooks/useLibraryUpdates'
import { HOME_ACCENT_LINK_CLASS } from './HomeCityShowsLink'

const HEADING_ID = 'home-library-updates-heading'

/**
 * "From your library" (PSY-2589, Figma `2168:100` / `2185:1392`): the
 * viewer's five newest inbox updates about the artists and venues they follow,
 * read or unread, newest first.
 *
 * It reads the inbox rather than the catalog, so each follow's alert scope and
 * on/off switch apply here exactly as they do to the bell: one truth for bell,
 * inbox and Home.
 *
 * Renders NOTHING when there is nothing to show, with one exception (see
 * {@link HomeAreaPrompt}). A failed or pending read also renders nothing: the
 * server prefetches the rows, so a pending read is a client-only edge, and an
 * empty state that apologises is worse than an absent module.
 */
export function LibraryUpdates() {
  const { authStatus } = useAuthContext()
  const updates = useLibraryUpdates()

  if (authStatus === 'anonymous' || !updates) return null
  if (updates.length === 0) return <HomeAreaPrompt />

  return (
    <LibraryUpdatesSection
      action={
        <Link href="/notifications" className={cn(HOME_ACCENT_LINK_CLASS, 'shrink-0')}>
          All updates →
        </Link>
      }
    >
      <ul className="flex flex-col border-t border-border">
        {updates.map(update => (
          <LibraryUpdateRow key={update.id} update={update} />
        ))}
      </ul>
    </LibraryUpdatesSection>
  )
}

function LibraryUpdatesSection({
  action,
  children,
}: {
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section aria-labelledby={HEADING_ID} className="flex w-full flex-col gap-3">
      <div className="flex items-baseline justify-between gap-x-4">
        <h2
          id={HEADING_ID}
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          From your library
        </h2>
        {action}
      </div>
      {children}
    </section>
  )
}

/**
 * One update. The whole row is the link; tapping an unread row marks THAT
 * notification read and navigates in the same click. The href is same-origin
 * relative, so the navigation stays client-side and the mark-read request is
 * not cancelled by a full page load.
 */
export function LibraryUpdateRow({ update }: { update: LibraryUpdate }) {
  const markRead = useMarkNotificationsRead()
  const { copy, unread } = update

  return (
    <li>
      <Link
        href={copy.href}
        onClick={() => {
          if (unread) markRead.mutate([update.id])
        }}
        // The show row's grid, so the kind labels line up under the date
        // column of the show rows around this module.
        className={cn(SHOW_ROW_GRID, 'group transition-colors hover:bg-muted/40')}
      >
        <span className="font-data text-xs font-bold uppercase text-muted-foreground">
          {copy.label}
        </span>
        <span className="min-w-0">
          <span className="block truncate text-sm font-medium leading-tight text-foreground transition-colors group-hover:text-link md:text-[15px]">
            {copy.title}
          </span>
          <span className="mt-0.5 block truncate text-xs text-muted-foreground md:text-[13px]">
            {copy.detail}
          </span>
        </span>
        <span className="flex items-center gap-2.5">
          {unread && (
            <span className="h-2 w-2 rounded-full bg-primary">
              <span className="sr-only">Unread</span>
            </span>
          )}
          <ChevronRight className="h-4 w-4 text-foreground" aria-hidden />
        </span>
      </Link>
    </li>
  )
}

/**
 * The one non-empty empty state (owner decision): a viewer who follows artists
 * but has no home area gets no artist show alerts at all, because an artist
 * follow's default near-me scope has nothing to be near. Saying nothing would
 * hide why the module is empty, so it says how to fix it.
 *
 * Its reads mount only once the update list is known to be empty, and it
 * renders nothing while either is unknown: a guessed "no home area" would
 * tell a viewer who has one to set it.
 */
function HomeAreaPrompt() {
  const { data: counts } = useLibraryFollowingCounts()
  const followsArtists = (counts?.artists ?? 0) > 0
  const hasHomeArea = useHomeMetroState(followsArtists)

  if (!followsArtists || hasHomeArea !== false) return null

  return (
    <LibraryUpdatesSection>
      <p className="border-t border-border pt-4">
        <Link href={ALERTS_AREA_HREF} className={HOME_ACCENT_LINK_CLASS}>
          Set your area to see new shows from artists you follow →
        </Link>
      </p>
    </LibraryUpdatesSection>
  )
}
