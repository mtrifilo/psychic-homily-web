'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { ArrowLeft, Hash, Loader2, Music, MapPin, Calendar, Disc3, Tag, Tent, Clock } from 'lucide-react'
import { NotifyMeButton } from '@/features/notifications'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb } from '@/components/shared'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useTag, useTagEntities } from '../hooks'
import { getCategoryColor, getCategoryLabel, getEntityUrl, getEntityTypePluralLabel } from '../types'
import type { TaggedEntityItem } from '../types'

interface TagDetailProps {
  slug: string
}

export function TagDetail({ slug }: TagDetailProps) {
  const { data: tag, isLoading, error } = useTag(slug)

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

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
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
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-3xl font-bold tracking-tight">{tag.name}</h1>
              {tag.is_official && (
                <Badge variant="secondary">Official</Badge>
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
              {tag.created_by_username && (
                <>
                  <span className="text-muted-foreground/40">{'·'}</span>
                  <span className="text-sm text-muted-foreground">
                    Created by{' '}
                    <Link
                      href={`/users/${tag.created_by_username}`}
                      className="hover:underline"
                    >
                      @{tag.created_by_username}
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

            {tag.description && (
              <p className="text-muted-foreground whitespace-pre-line max-w-2xl">
                {tag.description}
              </p>
            )}
          </div>
        </div>
      </header>

      {/* Metadata cards */}
      {((tag.parent_id && tag.parent_name) || tag.child_count > 0 || (tag.aliases && tag.aliases.length > 0)) && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
          {/* Parent tag */}
          {tag.parent_id && tag.parent_name && (
            <div className="rounded-lg border border-border/50 p-4">
              <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                Parent Tag
              </h2>
              <Link
                href={`/tags/${tag.parent_id}`}
                className="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm border border-border/50 hover:bg-muted/50 transition-colors"
              >
                <Hash className="h-3.5 w-3.5 text-muted-foreground" />
                {tag.parent_name}
              </Link>
            </div>
          )}

          {/* Child tags count */}
          {tag.child_count > 0 && (
            <div className="rounded-lg border border-border/50 p-4">
              <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                Sub-tags
              </h2>
              <p className="text-sm text-muted-foreground">
                {tag.child_count} {tag.child_count === 1 ? 'sub-tag' : 'sub-tags'}
              </p>
            </div>
          )}

          {/* Aliases */}
          {tag.aliases && tag.aliases.length > 0 && (
            <div className="rounded-lg border border-border/50 p-4 sm:col-span-2">
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
          )}
        </div>
      )}

      {/* Usage Stats + Tagged Entities */}
      {tag.usage_count > 0 && (
        <TaggedEntitiesSection slug={slug} />
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Tagged entities section
// ──────────────────────────────────────────────

const ENTITY_TYPE_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  artist: Music,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/** Display order for entity type groups */
const ENTITY_TYPE_ORDER = ['artist', 'venue', 'show', 'release', 'label', 'festival']

function TaggedEntitiesSection({ slug }: { slug: string }) {
  const { data, isLoading } = useTagEntities(slug, { limit: 200 })

  const grouped = useMemo(() => {
    if (!data?.entities) return {}
    const groups: Record<string, TaggedEntityItem[]> = {}
    for (const entity of data.entities) {
      if (!groups[entity.entity_type]) {
        groups[entity.entity_type] = []
      }
      groups[entity.entity_type].push(entity)
    }
    return groups
  }, [data?.entities])

  const sortedTypes = useMemo(() => {
    return ENTITY_TYPE_ORDER.filter((t) => grouped[t]?.length)
  }, [grouped])

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
    <section className="border-t border-border/50 pt-6">
      <h2 className="text-lg font-semibold mb-4">Tagged Entities</h2>

      {/* Usage breakdown by entity type */}
      {sortedTypes.length > 1 && (
        <div className="flex flex-wrap gap-3 mb-6">
          {sortedTypes.map((entityType) => {
            const count = grouped[entityType].length
            const Icon = ENTITY_TYPE_ICONS[entityType] || Hash
            return (
              <div
                key={entityType}
                className="inline-flex items-center gap-2 rounded-md border border-border/50 bg-muted/30 px-3 py-1.5 text-sm"
              >
                <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="font-medium">{count}</span>
                <span className="text-muted-foreground">
                  {count === 1
                    ? entityType.charAt(0).toUpperCase() + entityType.slice(1)
                    : getEntityTypePluralLabel(entityType)}
                </span>
              </div>
            )
          })}
        </div>
      )}

      <div className="space-y-6">
        {sortedTypes.map((entityType) => {
          const entities = grouped[entityType]
          const Icon = ENTITY_TYPE_ICONS[entityType] || Hash
          return (
            <div key={entityType}>
              <h3 className="flex items-center gap-2 text-sm font-medium text-muted-foreground mb-2">
                <Icon className="h-4 w-4" />
                {getEntityTypePluralLabel(entityType)}
                <span className="text-xs">({entities.length})</span>
              </h3>
              <ul className="grid gap-1">
                {entities.map((entity) => (
                  <li key={`${entity.entity_type}-${entity.entity_id}`}>
                    <Link
                      href={getEntityUrl(entity.entity_type, entity.slug)}
                      className="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm hover:bg-muted/50 transition-colors"
                    >
                      {entity.name}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          )
        })}
      </div>
    </section>
  )
}
