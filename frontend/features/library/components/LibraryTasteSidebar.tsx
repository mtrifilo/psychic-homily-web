'use client'

import Link from 'next/link'
import { useAuthContext } from '@/lib/context/AuthContext'
import { usePersonalChartsStats } from '@/features/charts/hooks'
import type {
  PersonalChartsStats,
  PersonalTopArtist,
  PersonalTopScene,
  PersonalTopTag,
} from '@/features/charts/types'

function firstActivityLabel(value: string | null): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toLocaleDateString('en-US', {
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

function sceneLabel(scene: PersonalTopScene): string {
  const place = [scene.city, scene.state].filter(Boolean).join(', ')
  return place || scene.name
}

function rankLabel(index: number): string {
  return String(index + 1).padStart(2, '0')
}

function SidebarBlock({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="border-t border-border pt-3">
      <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-[0.04em] text-foreground">
        {title}
      </h3>
      {children}
    </section>
  )
}

function SnapshotBlock({ stats }: { stats: PersonalChartsStats }) {
  const collectingSince = firstActivityLabel(stats.first_activity_at)

  return (
    <SidebarBlock title="Snapshot">
      <dl className="space-y-0">
        <div className="flex items-baseline justify-between gap-3 border-b border-border/60 py-1.5">
          <dt className="text-[13px] text-muted-foreground">Saved shows</dt>
          <dd className="font-mono text-[13px] tabular-nums text-foreground">
            {stats.saved_shows}
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-3 border-b border-border/60 py-1.5">
          <dt className="text-[13px] text-muted-foreground">Artists followed</dt>
          <dd className="font-mono text-[13px] tabular-nums text-foreground">
            {stats.artists_followed}
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-3 border-b border-border/60 py-1.5">
          <dt className="text-[13px] text-muted-foreground">Top venue</dt>
          <dd className="truncate text-right text-[13px] text-foreground">
            {stats.top_venue ? (
              <Link
                href={`/venues/${stats.top_venue.slug}`}
                className="transition-colors hover:text-primary"
              >
                {stats.top_venue.name}
              </Link>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-3 py-1.5">
          <dt className="text-[13px] text-muted-foreground">Collecting since</dt>
          <dd className="font-mono text-[13px] tabular-nums text-foreground">
            {collectingSince ?? (
              <span className="text-muted-foreground">—</span>
            )}
          </dd>
        </div>
      </dl>
    </SidebarBlock>
  )
}

function TopScenesBlock({ scenes }: { scenes: PersonalTopScene[] }) {
  if (scenes.length === 0) return null

  return (
    <SidebarBlock title="Top scenes">
      <ol className="space-y-0">
        {scenes.map((scene, index) => (
          <li
            key={scene.slug || scene.metro}
            className="flex items-baseline gap-2 border-b border-border/60 py-1.5 last:border-b-0"
          >
            <span className="w-4 shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground">
              {rankLabel(index)}
            </span>
            <Link
              href={`/scenes/${scene.slug}`}
              className="min-w-0 flex-1 truncate text-[13px] text-foreground transition-colors hover:text-primary"
            >
              {sceneLabel(scene)}
            </Link>
            <span className="shrink-0 font-mono text-[12px] tabular-nums text-muted-foreground">
              {scene.count}
            </span>
          </li>
        ))}
      </ol>
    </SidebarBlock>
  )
}

function TopTagsBlock({ tags }: { tags: PersonalTopTag[] }) {
  if (tags.length === 0) return null

  return (
    <SidebarBlock title="Top tags">
      <ul className="flex flex-wrap gap-1.5 pt-1">
        {tags.map(tag => (
          <li key={tag.tag_id}>
            <Link
              href={`/tags/${tag.slug}`}
              className="inline-block border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground transition-colors hover:border-primary hover:text-primary"
            >
              {tag.name}
            </Link>
          </li>
        ))}
      </ul>
    </SidebarBlock>
  )
}

function TopArtistsBlock({ artists }: { artists: PersonalTopArtist[] }) {
  if (artists.length === 0) return null

  return (
    <SidebarBlock title="Top artists">
      <ol className="space-y-0">
        {artists.map((artist, index) => (
          <li
            key={artist.artist_id}
            className="flex items-baseline gap-2 border-b border-border/60 py-1.5 last:border-b-0"
          >
            <span className="w-4 shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground">
              {rankLabel(index)}
            </span>
            <Link
              href={`/artists/${artist.slug}`}
              className="min-w-0 flex-1 truncate text-[13px] text-foreground transition-colors hover:text-primary"
            >
              {artist.name}
            </Link>
            <span className="shrink-0 font-mono text-[12px] tabular-nums text-muted-foreground">
              {artist.count}
            </span>
          </li>
        ))}
      </ol>
    </SidebarBlock>
  )
}

function TasteSidebarBody({ stats }: { stats: PersonalChartsStats }) {
  return (
    <div className="space-y-5">
      <SnapshotBlock stats={stats} />
      <TopScenesBlock scenes={stats.top_scenes} />
      <TopTagsBlock tags={stats.top_tags} />
      <TopArtistsBlock artists={stats.top_artists} />
    </div>
  )
}

function TasteSidebarSkeleton() {
  return (
    <div
      aria-label="Loading your taste"
      className="space-y-4"
      data-testid="library-taste-sidebar-loading"
    >
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="space-y-2 border-t border-border pt-3">
          <div className="h-3 w-20 animate-pulse rounded-sm bg-muted" />
          <div className="h-16 w-full animate-pulse rounded-sm bg-muted/60" />
        </div>
      ))}
    </div>
  )
}

/**
 * Gazelle-style taste sidebar for /library (PSY-1429 State G).
 * Driven exclusively by GET /charts/me — no invented numbers.
 */
export function LibraryTasteSidebar() {
  const { isAuthenticated, isLoading: isAuthLoading, user } = useAuthContext()
  const stats = usePersonalChartsStats(
    user?.id,
    isAuthenticated && !isAuthLoading
  )

  if (!isAuthenticated) return null

  return (
    <aside
      aria-label="Your taste"
      className="w-full lg:w-80 lg:shrink-0"
      data-testid="library-taste-sidebar"
    >
      <p className="mb-3 font-mono text-[11px] font-bold uppercase tracking-[0.08em] text-muted-foreground">
        Your taste
      </p>
      {isAuthLoading || stats.isLoading ? (
        <TasteSidebarSkeleton />
      ) : stats.isError || !stats.data ? (
        <p className="border-t border-border pt-3 text-[13px] text-muted-foreground">
          Mark shows you&apos;re going to and this fills in.
        </p>
      ) : (
        <TasteSidebarBody stats={stats.data} />
      )}
    </aside>
  )
}
