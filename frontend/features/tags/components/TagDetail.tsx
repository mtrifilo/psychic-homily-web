'use client'

import { useCallback, useMemo, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { ArrowLeft, Hash, Loader2, X } from 'lucide-react'
import { NotifyMeButton } from '@/features/notifications'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Breadcrumb, FollowButton } from '@/components/shared'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useTagDetail, useTagIntersection, useSearchTags } from '../hooks'
import {
  getCategoryColor,
  getCategoryLabel,
  getTagSectionLabel,
  getTagSectionBrowseUrl,
  TAG_DETAIL_SECTION_ORDER,
} from '../types'
import type { TagIntersectionGroup, TagSummary } from '../types'
import { TagOfficialIndicator } from './TagOfficialIndicator'
import { TaggedEntityRow } from './TaggedEntityCards'

interface TagDetailProps {
  slug: string
}

/** Per-type preview size (rows shown before the "Show all" link). */
const SECTION_PREVIEW_LIMIT = 5

export function TagDetail({ slug }: TagDetailProps) {
  const { data: tag, isLoading, error } = useTagDetail(slug)

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

  return <TagDetailContent slug={slug} tag={tag} />
}

// ──────────────────────────────────────────────
// Loaded content — split out so hooks below can run unconditionally once the
// tag has resolved (the loading/error/!tag early returns live in TagDetail).
// ──────────────────────────────────────────────

function TagDetailContent({
  slug,
  tag,
}: {
  slug: string
  tag: NonNullable<ReturnType<typeof useTagDetail>['data']>
}) {
  const router = useRouter()
  const searchParams = useSearchParams()

  // Added-tag pivot state lives in the URL (?with=slug,…) so it's shareable and
  // degrades to the single-tag detail when dropped (PSY-995 decision #5). The
  // page's own slug is always the first tag of the intersection.
  const addedSlugs = useMemo(() => {
    const raw = searchParams.get('with')
    if (!raw) return []
    return raw
      .split(',')
      .map((s) => s.trim().toLowerCase())
      .filter((s) => s && s !== slug)
  }, [searchParams, slug])

  const intersectionSlugs = useMemo(
    () => [slug, ...addedSlugs],
    [slug, addedSlugs]
  )
  const isFiltering = addedSlugs.length > 0

  const setAddedSlugs = useCallback(
    (next: string[]) => {
      const params = new URLSearchParams(searchParams.toString())
      if (next.length > 0) params.set('with', next.join(','))
      else params.delete('with')
      const qs = params.toString()
      router.replace(qs ? `/tags/${slug}?${qs}` : `/tags/${slug}`, {
        scroll: false,
      })
    },
    [router, searchParams, slug]
  )

  const { data: intersection, isLoading: groupsLoading } = useTagIntersection(
    intersectionSlugs,
    { previewLimit: SECTION_PREVIEW_LIMIT },
    // Don't fire for a brand-new / unused tag with no entities — its sections
    // would all be empty and the endpoint resolves the slug anyway.
    { enabled: tag.usage_count > 0 }
  )

  // Reorder the backend's canonical groups into the design's fixed display
  // order, dropping zero-count sections (empty-suppression).
  const sections = useMemo(() => {
    const byType = new Map<string, TagIntersectionGroup>()
    for (const g of intersection?.groups ?? []) byType.set(g.entity_type, g)
    return TAG_DETAIL_SECTION_ORDER.map((t) => byType.get(t)).filter(
      (g): g is TagIntersectionGroup => Boolean(g && g.count > 0)
    )
  }, [intersection])

  // Sparse = exactly one non-empty section with a single item, and no active
  // pivot. Matches Figma frame 437:7 (a 1-use tag).
  const isSparse =
    !isFiltering &&
    sections.length === 1 &&
    sections[0].count === 1 &&
    !groupsLoading

  // Only genre tags participate in the parent/children hierarchy.
  const isGenre = tag.category === 'genre'
  const parentCrumbs =
    isGenre && tag.parent
      ? [
          {
            href: `/tags/${tag.parent.slug || tag.parent.id}`,
            label: tag.parent.name,
          },
        ]
      : undefined

  const creatorHandle = tag.created_by?.username || tag.created_by_username

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      <Breadcrumb
        fallback={{ href: '/tags', label: 'Tags' }}
        intermediate={parentCrumbs}
        currentPage={tag.name}
      />

      {/* ── Thin metadata band ───────────────────────────────────────────
          Title + category chip + actions lead; meta (uses · creator · added)
          and description are DEMOTED below, not the focus. */}
      <header className="mb-8 mt-2" data-testid="tag-header">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 min-w-0">
            <Hash className="h-6 w-6 shrink-0 text-muted-foreground" aria-hidden />
            <h1 className="text-3xl font-bold tracking-tight min-w-0 break-words">
              {tag.name}
            </h1>
            <span
              className={cn(
                'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
                getCategoryColor(tag.category)
              )}
            >
              {getCategoryLabel(tag.category)}
            </span>
            {tag.is_official && (
              <TagOfficialIndicator size="md" tagName={tag.name} />
            )}
          </div>

          <div className="flex shrink-0 items-center gap-2">
            <FollowButton entityType="tags" entityId={tag.id} />
            <NotifyMeButton
              entityType="tag"
              entityId={tag.id}
              entityName={tag.name}
            />
            <Button asChild variant="outline" size="sm">
              <Link href={`/shows?tags=${slug}`}>Filter shows</Link>
            </Button>
          </div>
        </div>

        {/* Demoted one-line meta. */}
        <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs text-muted-foreground">
          <span>
            {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
          </span>
          {creatorHandle && (
            <>
              <span className="text-muted-foreground/40" aria-hidden>
                ·
              </span>
              <span>
                created by{' '}
                <Link
                  href={`/users/${creatorHandle}`}
                  className="hover:underline"
                >
                  @{creatorHandle}
                </Link>
              </span>
            </>
          )}
          {tag.created_at && (
            <>
              <span className="text-muted-foreground/40" aria-hidden>
                ·
              </span>
              <span>added {formatRelativeTime(tag.created_at)}</span>
            </>
          )}
        </div>

        {/* Optional description. */}
        {tag.description_html ? (
          <div
            className="prose prose-sm dark:prose-invert max-w-2xl text-muted-foreground mt-3"
            data-testid="tag-description"
            dangerouslySetInnerHTML={{ __html: tag.description_html }}
          />
        ) : tag.description ? (
          <p className="text-muted-foreground whitespace-pre-line max-w-2xl mt-3">
            {tag.description}
          </p>
        ) : null}
      </header>

      {/* Active-pivot chip row: shows the added tags + a clear control. */}
      {isFiltering && intersection && (
        <ActiveFilterBar
          baseTag={{ slug, name: tag.name }}
          addedTags={intersection.tags.filter((t) => t.slug !== slug)}
          onRemove={(removeSlug) =>
            setAddedSlugs(addedSlugs.filter((s) => s !== removeSlug))
          }
          onClear={() => setAddedSlugs([])}
        />
      )}

      {/* ── Co-visible entity-type sections ─────────────────────────────── */}
      {groupsLoading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : sections.length > 0 ? (
        <div className="space-y-8" data-testid="tag-sections">
          {sections.map((group) => (
            <EntitySection
              key={group.entity_type}
              group={group}
              slugs={intersectionSlugs}
            />
          ))}
        </div>
      ) : (
        <EmptyIntersectionState isFiltering={isFiltering} tagName={tag.name} />
      )}

      {/* "Help grow this tag" CTA for sparse single-item tags (frame 437:7). */}
      {isSparse && <HelpGrowCta tagName={tag.name} />}

      {/* ── Related tags rail + add-a-tag pivot ─────────────────────────── */}
      <RelatedTagsRail
        relatedTags={tag.related_tags ?? []}
        activeSlugs={intersectionSlugs}
        onAddTag={(addSlug) => {
          const normalized = addSlug.toLowerCase()
          // Guard against re-adding an already-active tag (would write a
          // `?with=ambient,ambient` URL). The backend dedupes anyway, but a
          // clean URL keeps the chip UI honest.
          if (normalized === slug || addedSlugs.includes(normalized)) return
          setAddedSlugs([...addedSlugs, normalized])
        }}
      />
    </div>
  )
}

// ──────────────────────────────────────────────
// Entity-type section
// ──────────────────────────────────────────────

function EntitySection({
  group,
  slugs,
}: {
  group: TagIntersectionGroup
  slugs: string[]
}) {
  const label = getTagSectionLabel(group.entity_type)
  const showAllUrl = getTagSectionBrowseUrl(group.entity_type, slugs)

  return (
    <section data-testid={`tag-section-${group.entity_type}`}>
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <h2 className="flex items-baseline gap-2 text-lg font-semibold">
          {label}
          <span className="font-mono text-sm font-normal text-muted-foreground tabular-nums">
            {group.count}
          </span>
        </h2>
        {showAllUrl && (
          <Link
            href={showAllUrl}
            className="shrink-0 text-sm font-medium text-primary hover:underline"
            data-testid={`tag-section-showall-${group.entity_type}`}
          >
            Show all {group.count} &rarr;
          </Link>
        )}
      </div>
      <div>
        {group.preview.map((item) => (
          <TaggedEntityRow
            key={`${item.entity_type}-${item.entity_id}`}
            item={item}
          />
        ))}
      </div>
    </section>
  )
}

// ──────────────────────────────────────────────
// Active-filter bar (pivot chips)
// ──────────────────────────────────────────────

function ActiveFilterBar({
  baseTag,
  addedTags,
  onRemove,
  onClear,
}: {
  baseTag: { slug: string; name: string }
  addedTags: TagSummary[]
  onRemove: (slug: string) => void
  onClear: () => void
}) {
  return (
    <div
      className="mb-6 flex flex-wrap items-center gap-2 rounded-lg border border-border/50 bg-muted/20 p-3"
      data-testid="active-filter-bar"
    >
      <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        Filtering by
      </span>
      <span className="inline-flex items-center gap-1 rounded-md border border-border bg-background px-2 py-0.5 text-sm">
        <Hash className="h-3 w-3 opacity-60" aria-hidden />
        {baseTag.name}
      </span>
      {addedTags.map((t) => (
        <span
          key={t.slug}
          className="inline-flex items-center gap-1 rounded-md border border-primary/40 bg-primary/5 px-2 py-0.5 text-sm"
        >
          <Hash className="h-3 w-3 opacity-60" aria-hidden />
          {t.name}
          <button
            type="button"
            onClick={() => onRemove(t.slug)}
            className="ml-0.5 rounded-sm text-muted-foreground hover:text-foreground"
            aria-label={`Remove ${t.name} filter`}
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <button
        type="button"
        onClick={onClear}
        className="ml-1 text-xs text-muted-foreground hover:text-foreground hover:underline"
      >
        Clear
      </button>
    </div>
  )
}

// ──────────────────────────────────────────────
// Related-tags rail + "+ add another tag to filter" pivot
// ──────────────────────────────────────────────

function RelatedTagsRail({
  relatedTags,
  activeSlugs,
  onAddTag,
}: {
  relatedTags: TagSummary[]
  activeSlugs: string[]
  onAddTag: (slug: string) => void
}) {
  const [adding, setAdding] = useState(false)

  // Related tags already in the active intersection are not offerable.
  const offerable = relatedTags.filter((t) => !activeSlugs.includes(t.slug))

  if (offerable.length === 0 && !adding) {
    // Nothing to suggest and the picker is closed — still expose the pivot so
    // the user can search for any tag to intersect.
    return (
      <section
        className="mt-12 border-t border-border/50 pt-6"
        data-testid="related-tags"
      >
        <AddTagPivot
          activeSlugs={activeSlugs}
          onAddTag={onAddTag}
          open={adding}
          setOpen={setAdding}
        />
      </section>
    )
  }

  return (
    <section
      className="mt-12 border-t border-border/50 pt-6"
      data-testid="related-tags"
    >
      <h2 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        Related tags
      </h2>
      <div className="flex flex-wrap items-center gap-2">
        {offerable.map((t) => (
          <button
            key={t.slug}
            type="button"
            onClick={() => onAddTag(t.slug)}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors hover:bg-muted/50',
              getCategoryColor(t.category)
            )}
            data-testid={`related-tag-${t.slug}`}
          >
            <Hash className="h-3.5 w-3.5 opacity-70" aria-hidden />
            {t.name}
          </button>
        ))}
        <AddTagPivot
          activeSlugs={activeSlugs}
          onAddTag={onAddTag}
          open={adding}
          setOpen={setAdding}
        />
      </div>
    </section>
  )
}

/**
 * The "+ add another tag to filter" control. Closed = an orange-bordered button;
 * open = a small search-as-you-type picker over the tag corpus. Selecting a tag
 * narrows the page in place (the parent re-queries the intersection).
 */
function AddTagPivot({
  activeSlugs,
  onAddTag,
  open,
  setOpen,
}: {
  activeSlugs: string[]
  onAddTag: (slug: string) => void
  open: boolean
  setOpen: (open: boolean) => void
}) {
  const [query, setQuery] = useState('')
  const { data: results, isLoading } = useSearchTags(query.trim(), 8)

  const matches = (results?.tags ?? []).filter(
    (t) => !activeSlugs.includes(t.slug)
  )

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-1.5 rounded-md border border-primary/50 px-3 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/5"
        data-testid="add-tag-pivot-trigger"
      >
        + add another tag to filter
      </button>
    )
  }

  return (
    <div className="relative w-full max-w-xs" data-testid="add-tag-pivot-picker">
      <input
        autoFocus
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onBlur={() => {
          // Defer close so an option click registers first.
          window.setTimeout(() => setOpen(false), 150)
        }}
        placeholder="Search tags to filter…"
        aria-label="Search tags to filter"
        className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
      />
      {query.trim().length >= 2 && (
        <ul className="absolute z-10 mt-1 max-h-56 w-full overflow-auto rounded-md border border-border bg-popover py-1 shadow-md">
          {isLoading ? (
            <li className="px-3 py-2 text-sm text-muted-foreground">
              Searching…
            </li>
          ) : matches.length === 0 ? (
            <li className="px-3 py-2 text-sm text-muted-foreground">
              No matching tags
            </li>
          ) : (
            matches.map((t) => (
              <li key={t.slug}>
                <button
                  type="button"
                  // onMouseDown (not onClick) so it fires before the input's
                  // onBlur close.
                  onMouseDown={(e) => {
                    e.preventDefault()
                    onAddTag(t.slug)
                    setQuery('')
                    setOpen(false)
                  }}
                  className="flex w-full items-center gap-1.5 px-3 py-1.5 text-left text-sm hover:bg-muted/60"
                >
                  <Hash className="h-3.5 w-3.5 opacity-60" aria-hidden />
                  {t.name}
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Empty / sparse states
// ──────────────────────────────────────────────

function EmptyIntersectionState({
  isFiltering,
  tagName,
}: {
  isFiltering: boolean
  tagName: string
}) {
  return (
    <div
      className="rounded-lg border border-border/50 bg-muted/20 p-6 text-center"
      data-testid="empty-intersection-state"
    >
      <p className="text-muted-foreground">
        {isFiltering
          ? 'No entities match all of the selected tags.'
          : `Nothing is tagged ${tagName} yet.`}
      </p>
    </div>
  )
}

function HelpGrowCta({ tagName }: { tagName: string }) {
  return (
    <section
      className="mt-8 rounded-lg border border-border/50 bg-muted/20 p-4"
      data-testid="help-grow-cta"
    >
      <h2 className="font-semibold">Help grow this tag</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Be the first to tag a release, show, or venue with {tagName} — it takes
        seconds.
      </p>
      <Button asChild size="sm" className="mt-3">
        <Link href="/shows/submit">Suggest something &rarr;</Link>
      </Button>
    </section>
  )
}
