'use client'

import { Fragment, useState } from 'react'
import Link from 'next/link'
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, Tag, MapPin } from 'lucide-react'
import { useLabel, useLabelRoster, useLabelCatalog } from '../hooks/useLabels'
import {
  EntityDetailLayout,
  EntityHeader,
  SocialLinks,
  FollowButton,
  AddToCollectionButton,
  RevisionHistory,
  BracketLink,
  SectionHeader,
  StatsList,
  DenseTable,
  DenseTableGroupHeader,
  type StatsListItem,
} from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { CommentThread } from '@/features/comments'
import { EntityTagList, AddTagDialog } from '@/features/tags'
import { NotifyMeButton } from '@/features/notifications'
import { useIsAuthenticated } from '@/features/auth'
import { AttributionLine, ContributionPrompt, EntityEditDrawer, EntitySaveSuccessBanner, useEntitySaveSuccessBanner } from '@/features/contributions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { queryKeys } from '@/lib/queryClient'
import {
  getLabelStatusLabel,
  getLabelStatusVariant,
  formatLabelLocation,
  type LabelRelease,
} from '../types'
import { RELEASE_TYPES, type ReleaseType } from '@/features/releases/types'

const CATALOG_BUCKETS: Array<{
  key: ReleaseType | 'other'
  label: string
  match: (r: LabelRelease) => boolean
}> = [
  { key: 'lp', label: 'Albums', match: r => r.release_type === 'lp' },
  { key: 'ep', label: 'EPs', match: r => r.release_type === 'ep' },
  { key: 'single', label: 'Singles', match: r => r.release_type === 'single' },
  { key: 'compilation', label: 'Compilations', match: r => r.release_type === 'compilation' },
  { key: 'live', label: 'Live', match: r => r.release_type === 'live' },
  { key: 'remix', label: 'Remixes', match: r => r.release_type === 'remix' },
  { key: 'demo', label: 'Demos', match: r => r.release_type === 'demo' },
  { key: 'other', label: 'Other', match: r => !(RELEASE_TYPES as readonly string[]).includes(r.release_type) },
]

interface LabelDetailProps {
  idOrSlug: string | number
}

export function LabelDetail({ idOrSlug }: LabelDetailProps) {
  const { data: label, isLoading, error } = useLabel({ idOrSlug })
  const { user, isAuthenticated } = useIsAuthenticated()
  const queryClient = useQueryClient()
  const canEditDirectly = isAuthenticated && (
    user?.is_admin ||
    user?.user_tier === 'trusted_contributor' ||
    user?.user_tier === 'local_ambassador'
  )
  const { data: rosterData, isLoading: rosterLoading } = useLabelRoster({
    labelIdOrSlug: idOrSlug,
    enabled: !!label,
  })
  const { data: catalogData, isLoading: catalogLoading } = useLabelCatalog({
    labelIdOrSlug: idOrSlug,
    enabled: !!label,
  })
  const [isEditing, setIsEditing] = useState(false)
  const [editFocusField, setEditFocusField] = useState<string | undefined>()
  const [addTagDialogOpen, setAddTagDialogOpen] = useState(false)
  const saveBanner = useEntitySaveSuccessBanner()

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load label'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Label Not Found' : 'Error Loading Label'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The label you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/labels">Back to Labels</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!label) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Label Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The label you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/labels">Back to Labels</Link>
          </Button>
        </div>
      </div>
    )
  }

  const location = formatLabelLocation(label)
  const roster = rosterData?.artists ?? []
  const catalog = catalogData?.releases ?? []
  // PSY-481 polish: `label.social` can be a non-null object whose every
  // value is empty/null (the API still returns the wrapper for labels with
  // no real links). Compute whether at least one link is present so we can
  // suppress the "Links" heading when SocialLinks would render nothing —
  // otherwise the detail page shows an orphan section header underlined by
  // empty space.
  const hasDescription = !!label.description && label.description.trim().length > 0
  const hasSocialLinks = !!(
    label.social &&
    Object.values(label.social).some(value => typeof value === 'string' && value.trim() !== '')
  )

  const statsItems: StatsListItem[] = [
    { label: 'Roster', value: label.artist_count },
    { label: 'Catalog', value: label.release_count },
  ]
  if (label.founded_year) {
    // String() bypasses StatsList's Intl.NumberFormat thousands separator —
    // 1980 should render as "1980", not "1,980".
    statsItems.push({ label: 'Founded', value: String(label.founded_year) })
  }
  statsItems.push({
    label: 'Status',
    value: (
      <Badge variant={getLabelStatusVariant(label.status)} className="text-[10px] px-1.5 py-0">
        {getLabelStatusLabel(label.status)}
      </Badge>
    ),
  })

  const catalogGroups = CATALOG_BUCKETS.map(b => ({
    ...b,
    releases: catalog.filter(b.match),
  })).filter(g => g.releases.length > 0)

  const sidebar = (
    <div className="space-y-6">
      {label.image_url ? (
        <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
          <img
            src={label.image_url}
            alt={`${label.name} logo`}
            className="w-full aspect-square object-cover"
          />
        </div>
      ) : (
        <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
          <div className="w-full aspect-square bg-muted/30 flex items-center justify-center">
            <Tag className="h-16 w-16 text-muted-foreground/30" />
          </div>
        </div>
      )}

      <section>
        <SectionHeader title="Statistics" />
        <StatsList items={statsItems} />
      </section>

      <EntityCollections entityType="label" entityId={label.id} />
    </div>
  )

  return (
    <>
    <EntityDetailLayout
      fallback={{ href: '/labels', label: 'Labels' }}
      entityName={label.name}
      header={
        <>
          <EntityHeader
            title={label.name}
            subtitle={
              <>
                <Badge variant={getLabelStatusVariant(label.status)}>
                  {getLabelStatusLabel(label.status)}
                </Badge>
                {location && (
                  <span className="flex items-center gap-1">
                    <MapPin className="h-3.5 w-3.5" />
                    {location}
                  </span>
                )}
                {label.founded_year && <span>Est. {label.founded_year}</span>}
              </>
            }
            actions={
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <FollowButton
                  entityType="labels"
                  entityId={label.id}
                  variant="bracket"
                />
                <NotifyMeButton
                  entityType="label"
                  entityId={label.id}
                  entityName={label.name}
                  variant="bracket"
                />
                <AddToCollectionButton
                  entityType="label"
                  entityId={label.id}
                  entityName={label.name}
                  variant="bracket"
                />
                {isAuthenticated && (
                  <BracketLink
                    label={canEditDirectly ? 'Edit' : 'Suggest edit'}
                    onClick={() => setIsEditing(true)}
                  />
                )}
                {isAuthenticated && !hasDescription && (
                  <BracketLink
                    label="Suggest description"
                    onClick={() => {
                      setEditFocusField('description')
                      setIsEditing(true)
                    }}
                  />
                )}
                {isAuthenticated && (
                  <BracketLink
                    label="Add tag"
                    onClick={() => setAddTagDialogOpen(true)}
                  />
                )}
              </div>
            }
          />
          <EntitySaveSuccessBanner visible={saveBanner.isVisible} />
          <AttributionLine entityType="label" entityId={label.id} />
          <EntityTagList
            entityType="label"
            entityId={label.id}
            isAuthenticated={isAuthenticated}
          />
          <ContributionPrompt
            entityType="label"
            entityId={label.id}
            entitySlug={label.slug}
            isAuthenticated={!!isAuthenticated}
            onEditClick={(focusField) => {
              setEditFocusField(focusField)
              setIsEditing(true)
            }}
          />
        </>
      }
      sidebar={sidebar}
    >
      <div className="space-y-8">
        {hasDescription && (
          <section>
            <SectionHeader title="About" as="h2" size="md" />
            <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
              {label.description}
            </p>
          </section>
        )}

        {hasSocialLinks && (
          <section>
            <SectionHeader title="Links" as="h2" size="md" />
            <SocialLinks social={label.social} />
          </section>
        )}

        {rosterLoading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : roster.length > 0 ? (
          <section>
            <SectionHeader title="Roster" as="h2" size="md" />
            <ul className="space-y-1 text-sm">
              {roster.map(artist => (
                <li key={artist.id}>
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="text-foreground hover:text-primary hover:underline underline-offset-2 transition-colors"
                  >
                    {artist.name}
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {catalogLoading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : catalogGroups.length > 0 ? (
          <section>
            <SectionHeader title="Catalog" as="h2" size="md" />
            <DenseTable variant="alternating" aria-label="Catalog">
              <thead>
                <tr>
                  <th>Title</th>
                  <th className="text-right">Year</th>
                  <th>Catalog #</th>
                </tr>
              </thead>
              <tbody>
                {catalogGroups.map(g => (
                  <Fragment key={g.key}>
                    <DenseTableGroupHeader title={g.label} colSpan={3} />
                    {g.releases.map(r => (
                      <tr key={r.id}>
                        <td>
                          <Link
                            href={`/releases/${r.slug}`}
                            className="hover:text-primary underline-offset-2 hover:underline"
                          >
                            {r.title}
                          </Link>
                        </td>
                        <td className="text-right text-muted-foreground">
                          {r.release_year ?? '—'}
                        </td>
                        <td className="text-muted-foreground">
                          {r.catalog_number ?? '—'}
                        </td>
                      </tr>
                    ))}
                  </Fragment>
                ))}
              </tbody>
            </DenseTable>
          </section>
        ) : null}
      </div>
    </EntityDetailLayout>

    {/* Revision History */}
    <div className="mt-0">
      <RevisionHistory entityType="label" entityId={label.id} />
    </div>

    {/* Discussion */}
    <div className="mt-0 px-4 md:px-0">
      <CommentThread entityType="label" entityId={label.id} />
    </div>

    {/* Edit Drawer (all authenticated users) */}
    {isAuthenticated && (
      <EntityEditDrawer
        open={isEditing}
        onOpenChange={(open) => {
          setIsEditing(open)
          if (!open) setEditFocusField(undefined)
        }}
        entityType="label"
        entityId={label.id}
        entityName={label.name}
        entity={label as unknown as Record<string, unknown>}
        canEditDirectly={!!canEditDirectly}
        focusField={editFocusField}
        onSuccess={(result) => {
          queryClient.invalidateQueries({
            queryKey: queryKeys.labels.detail(idOrSlug),
          })
          saveBanner.handleSaveSuccess(result)
        }}
      />
    )}

    {isAuthenticated && (
      <AddTagDialog
        entityType="label"
        entityId={label.id}
        open={addTagDialogOpen}
        onOpenChange={setAddTagDialogOpen}
      />
    )}
  </>
  )
}
