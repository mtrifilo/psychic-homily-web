'use client'

import { useMemo, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Hash, Loader2, Music, MapPin, Calendar, Disc3, Tag, Tent, Clock } from 'lucide-react'
import { NotifyMeButton } from '@/features/notifications'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Breadcrumb } from '@/components/shared'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useTagDetail, useTagEntities } from '../hooks'
import { getCategoryColor, getCategoryLabel, getEntityTypePluralLabel } from '../types'
import type { TaggedEntityItem, TagSummary } from '../types'
import { TagOfficialIndicator } from './TagOfficialIndicator'
import { TaggedEntityCard } from './TaggedEntityCards'

interface TagDetailProps {
  slug: string
}

/** Entity type display order — genre tags use parent/children nav; others are flat. */
const ENTITY_TYPE_ORDER = ['artist', 'venue', 'show', 'release', 'label', 'festival'] as const

const ENTITY_TYPE_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  artist: Music,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/** Singular display label for entity types used in the breakdown row. */
function getEntityTypeSingularLabel(entityType: string): string {
  switch (entityType) {
    case 'artist':
      return 'artist'
    case 'venue':
      return 'venue'
    case 'show':
      return 'show'
    case 'release':
      return 'release'
    case 'label':
      return 'label'
    case 'festival':
      return 'festival'
    default:
      return entityType
  }
}

export function TagDetail({ slug }: TagDetailProps) {
  const { data: tag, isLoading, error } = useTagDetail(slug)

  // Usage breakdown: only show non-zero counts. We pad with zeros on the backend
  // so the object always has all keys, but displaying zero counts is noise.
  // NOTE: hook must be called unconditionally, above the early returns below.
  // Guard the logic inside rather than the hook call.
  const breakdownEntries = useMemo(() => {
    if (!tag) return []
    return ENTITY_TYPE_ORDER
      .map((type) => ({ type, count: tag.usage_breakdown?.[type] ?? 0 }))
      .filter((e) => e.count > 0)
  }, [tag])

  // Top contributors: hide anonymous contributors (no username). Rendering
  // `user #{id}` leaks an internal DB row id and reads like placeholder content
  // (PSY-450). Contributors without a public username haven't opted in to
  // public attribution anyway.
  const visibleContributors = useMemo(() => {
    if (!tag?.top_contributors) return []
    return tag.top_contributors.filter((c) => Boolean(c.user.username))
  }, [tag])

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load tag'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Tag Not Found' : 'Error Loading Tag'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The tag you're looking for doesn't exist."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/tags">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Tags
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!tag) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Tag Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The tag you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/tags">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Tags
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  // Only genre tags participate in the parent/children hierarchy; other
  // categories are flat per the tag system design doc.
  const isGenre = tag.category === 'genre'
  const hasParent = isGenre && Boolean(tag.parent)
  const hasChildren = isGenre && tag.children && tag.children.length > 0

  // Build the parent breadcrumb chain. The detail endpoint exposes only the
  // direct parent (not the full ancestor chain) — see backend `GetTagDetail`
  // — so we render at most one intermediate crumb. If we ever expose
  // ancestors[] we can map them here without changing the Breadcrumb API.
  // Non-genre categories don't participate in the hierarchy, so skip the
  // intermediate crumb even if a stray parent_id were ever present.
  const parentCrumbs =
    isGenre && tag.parent
      ? [{ href: `/tags/${tag.parent.slug || tag.parent.id}`, label: tag.parent.name }]
      : undefined

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        intermediate={parentCrumbs}
        currentPage={tag.name}
      />

      {/* Header */}
      <header className="mb-8">
        <div className="flex items-start gap-4">
          <div
            className={cn(
              'mt-1 flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border',
              getCategoryColor(tag.category)
            )}
          >
            <Hash className="h-6 w-6" />
          </div>
          <div className="min-w-0 flex-1">
            {/* ISSUE-003 (dogfood tags-audit-4): at 375px the h1 + Official
                badge + NotifyMeButton cluster was clipped ~31px off-screen.
                flex-wrap + min-w-0 lets the NotifyMeButton break to a new
                row below the title on narrow viewports; desktop (>=sm)
                keeps the single-row layout. Same pattern as PSY-467. */}
            <div className="flex flex-wrap items-center gap-x-3 gap-y-2 min-w-0 mb-1">
              <h1 className="text-3xl font-bold tracking-tight min-w-0 break-words">{tag.name}</h1>
              {tag.is_official && (
                <TagOfficialIndicator size="md" tagName={tag.name} />
              )}
              <NotifyMeButton entityType="tag" entityId={tag.id} entityName={tag.name} />
            </div>

            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mb-4">
              <span
                className={cn(
                  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
                  getCategoryColor(tag.category)
                )}
              >
                {getCategoryLabel(tag.category)}
              </span>
              <span className="text-sm text-muted-foreground">
                {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
              </span>
              {(tag.created_by?.username || tag.created_by_username) && (
                <>
                  <span className="text-muted-foreground/40">{'·'}</span>
                  <span className="text-sm text-muted-foreground">
                    Created by{' '}
                    <Link
                      href={`/users/${tag.created_by?.username || tag.created_by_username}`}
                      className="hover:underline"
                    >
                      @{tag.created_by?.username || tag.created_by_username}
                    </Link>
                  </span>
                </>
              )}
              {tag.created_at && (
                <>
                  <span className="text-muted-foreground/40">{'·'}</span>
                  <span className="inline-flex items-center gap-1 text-sm text-muted-foreground">
                    <Clock className="h-3 w-3" />
                    {formatRelativeTime(tag.created_at)}
                  </span>
                </>
              )}
            </div>

            {/* Usage breakdown summary row: "15 artists · 0 venues · 3 releases" */}
            {breakdownEntries.length > 0 && (
              <div
                className="flex flex-wrap items-center gap-x-2 gap-y-1 mb-4 text-sm text-muted-foreground"
                data-testid="usage-breakdown-summary"
              >
                {breakdownEntries.map((entry, idx) => (
                  <span key={entry.type} className="inline-flex items-center gap-1">
                    {idx > 0 && <span className="text-muted-foreground/40">{'·'}</span>}
                    <span className="font-medium text-foreground">{entry.count}</span>
                    <span>
                      {entry.count === 1
                        ? getEntityTypeSingularLabel(entry.type)
                        : getEntityTypePluralLabel(entry.type).toLowerCase()}
                    </span>
                  </span>
                ))}
              </div>
            )}

            {/* Description: rendered markdown (goldmark + bluemonday via backend). */}
            {tag.description_html ? (
              <div
                className="prose prose-sm dark:prose-invert max-w-2xl text-muted-foreground"
                data-testid="tag-description"
                dangerouslySetInnerHTML={{ __html: tag.description_html }}
              />
            ) : tag.description ? (
              <p className="text-muted-foreground whitespace-pre-line max-w-2xl">
                {tag.description}
              </p>
            ) : null}
          </div>
        </div>
      </header>

      {/* Parent / Children hierarchy — genre tags only */}
      {(hasParent || hasChildren) && (
        <section
          className="mb-8 rounded-lg border border-border/50 p-4 space-y-3"
          data-testid="tag-hierarchy"
        >
          {hasParent && tag.parent && (
            <HierarchyRow label="Parent">
              <TagPill tag={tag.parent} />
            </HierarchyRow>
          )}
          {hasChildren && (
            <HierarchyRow label={`Children (${tag.children.length})`}>
              <div className="flex flex-wrap gap-2">
                {tag.children.map((c) => (
                  <TagPill key={c.id} tag={c} />
                ))}
              </div>
            </HierarchyRow>
          )}
        </section>
      )}

      {/* Metadata cards: aliases. Parent/children moved to the hierarchy row above. */}
      {tag.aliases && tag.aliases.length > 0 && (
        <div className="mb-8">
          <div className="rounded-lg border border-border/50 p-4">
            <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
              Also known as
            </h2>
            <div className="flex flex-wrap gap-2">
              {tag.aliases.map((alias: string) => (
                <span
                  key={alias}
                  className="inline-flex items-center rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground border border-border/50"
                >
                  {alias}
                </span>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Top contributors — anonymous contributors (no username) are hidden;
          see PSY-450. If every contributor is anonymous, the section is hidden
          entirely rather than showing an empty header. */}
      {visibleContributors.length > 0 && (
        <section className="mb-8" data-testid="top-contributors">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Top contributors
          </h2>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
            {visibleContributors.map((c, idx) => {
              const handle = c.user.username as string
              return (
                <span key={c.user.id} className="inline-flex items-center gap-1">
                  {idx > 0 && <span className="text-muted-foreground/40">{'·'}</span>}
                  <Link
                    href={`/users/${handle}`}
                    className="text-foreground hover:underline"
                  >
                    @{handle}
                  </Link>
                  <span className="text-muted-foreground">({c.count})</span>
                </span>
              )
            })}
          </div>
        </section>
      )}

      {/* Related tags pill row */}
      {tag.related_tags && tag.related_tags.length > 0 && (
        <section className="mb-8" data-testid="related-tags">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Related tags
          </h2>
          <div className="flex flex-wrap gap-2">
            {tag.related_tags.map((t) => (
              <TagPill key={t.id} tag={t} />
            ))}
          </div>
        </section>
      )}

      {/* Usage Stats + Tagged Entities — preserved from the original layout */}
      {tag.usage_count > 0 && (
        <TaggedEntitiesSection slug={slug} />
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Small presentational helpers
// ──────────────────────────────────────────────

function HierarchyRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider min-w-[72px] mt-1.5">
        {label}
      </span>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TagPill({ tag }: { tag: TagSummary }) {
  return (
    <Link
      href={`/tags/${tag.slug || tag.id}`}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-muted/50',
        getCategoryColor(tag.category)
      )}
    >
      <Hash className="h-3.5 w-3.5 opacity-70" />
      {tag.name}
      {tag.is_official && (
        <span className="text-[10px] font-medium uppercase tracking-wider opacity-70">
          official
        </span>
      )}
    </Link>
  )
}

// ──────────────────────────────────────────────
// Tagged entities section (PSY-485 — tabs + entity cards)
// ──────────────────────────────────────────────

function TaggedEntitiesSection({ slug }: { slug: string }) {
  const { data, isLoading } = useTagEntities(slug, { limit: 200 })

  const entities = data?.entities
  const grouped = useMemo(() => {
    if (!entities) return {}
    const groups: Record<string, TaggedEntityItem[]> = {}
    for (const entity of entities) {
      if (!groups[entity.entity_type]) {
        groups[entity.entity_type] = []
      }
      groups[entity.entity_type].push(entity)
    }
    return groups
  }, [entities])

  // Hide entity types with zero items so a genre-only tag doesn't render an
  // empty Festivals tab (PSY-485 acceptance criterion).
  const sortedTypes = useMemo(() => {
    return ENTITY_TYPE_ORDER.filter((t) => grouped[t]?.length)
  }, [grouped])

  // Default the active tab to the first non-empty entity type. We can't
  // recompute this in render because Radix Tabs is a controlled component —
  // need a real piece of state. The state is initialised lazily so it picks
  // up the first available type once entities load.
  const [activeTab, setActiveTab] = useState<string | undefined>(undefined)
  const effectiveTab =
    activeTab && sortedTypes.includes(activeTab as (typeof sortedTypes)[number])
      ? activeTab
      : sortedTypes[0]

  if (isLoading) {
    return (
      <section className="border-t border-border/50 pt-6">
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      </section>
    )
  }

  if (sortedTypes.length === 0) {
    return null
  }

  return (
    <section className="border-t border-border/50 pt-6" data-testid="tagged-entities">
      <h2 className="text-lg font-semibold mb-4">Tagged Entities</h2>

      <Tabs
        value={effectiveTab}
        onValueChange={setActiveTab}
        className="w-full"
      >
        {/* Tabs are scrollable on narrow viewports — six entity types of
            "Artists/Shows/Venues/..." would push past 375px otherwise. */}
        <div className="overflow-x-auto -mx-4 px-4 sm:mx-0 sm:px-0">
          <TabsList className="h-auto flex flex-wrap gap-1 bg-muted/40 p-1">
            {sortedTypes.map((entityType) => {
              const count = grouped[entityType].length
              const Icon = ENTITY_TYPE_ICONS[entityType] || Hash
              return (
                <TabsTrigger
                  key={entityType}
                  value={entityType}
                  data-testid={`tagged-entities-tab-${entityType}`}
                  className="gap-1.5"
                >
                  <Icon className="h-3.5 w-3.5" />
                  <span>{getEntityTypePluralLabel(entityType)}</span>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {count}
                  </span>
                </TabsTrigger>
              )
            })}
          </TabsList>
        </div>

        {sortedTypes.map((entityType) => {
          const entities = grouped[entityType]
          return (
            <TabsContent
              key={entityType}
              value={entityType}
              data-testid={`tagged-entities-panel-${entityType}`}
              className="mt-4"
            >
              <div className="grid gap-3">
                {entities.map((entity) => (
                  <TaggedEntityCard
                    key={`${entity.entity_type}-${entity.entity_id}`}
                    item={entity}
                  />
                ))}
              </div>
            </TabsContent>
          )
        })}
      </Tabs>
    </section>
  )
}
