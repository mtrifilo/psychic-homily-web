'use client'

import { useState } from 'react'
import {
  useCollections,
  useCollection,
  useCollectionStats,
  useSetFeatured,
  useDeleteCollection,
} from '../hooks'
import type { Collection } from '../types'
import { Switch } from '@/components/ui/switch'
import { EntityTypeBadge } from '@/components/shared'
import { AdminTable, type AdminTableColumn } from '@/components/admin/AdminTable'

/**
 * PSY-1504: build the "featured since {date}" caption shown beside the Featured
 * toggle for currently-featured collections. Data comes from the PSY-1500 admin
 * list payload (`featured_at` + `featured_at_estimated`) — no per-row fetch.
 *
 * - Returns `null` when there's no usable start date (unfeatured rows, or a
 *   featured row whose `featured_at` is missing/unparseable). The caller renders
 *   nothing in that case — no em-dash / placeholder.
 * - When the start was reconstructed at backfill (`estimated`), the date is
 *   presented as approximate ("featured since before {date}") so an estimated
 *   start is never shown as a precise, fabricated date.
 */
export function featuredSinceLabel(
  featuredAt?: string | null,
  estimated?: boolean | null
): string | null {
  if (!featuredAt) return null
  const date = new Date(featuredAt)
  if (Number.isNaN(date.getTime())) return null
  const formatted = date.toLocaleDateString()
  return estimated
    ? `featured since before ${formatted}`
    : `featured since ${formatted}`
}

function CollectionDetailPanel({
  collection,
  onClose,
}: {
  collection: Collection
  onClose: () => void
}) {
  const { data: detail, isLoading: detailLoading } = useCollection(collection.slug)
  const { data: stats, isLoading: statsLoading } = useCollectionStats(collection.slug)
  const deleteCollection = useDeleteCollection()

  const handleDelete = () => {
    if (
      window.confirm(
        `Delete collection "${collection.title}"? This cannot be undone.`
      )
    ) {
      deleteCollection.mutate(
        { slug: collection.slug },
        { onSuccess: () => onClose() }
      )
    }
  }

  return (
    <div className="border border-border rounded-lg p-4 space-y-4 bg-card">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">{collection.title}</h3>
          <p className="text-sm text-muted-foreground">
            /{collection.slug} &middot; by {collection.creator_name}
          </p>
        </div>
        <button
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground text-sm"
        >
          Close
        </button>
      </div>

      {collection.description && (
        <p className="text-sm text-muted-foreground">{collection.description}</p>
      )}

      {/* Stats */}
      <div>
        <h4 className="text-sm font-medium mb-2">Stats</h4>
        {statsLoading ? (
          <p className="text-sm text-muted-foreground">Loading stats...</p>
        ) : stats ? (
          <div className="space-y-2">
            <div className="grid grid-cols-3 gap-2 text-sm">
              <div className="text-center p-2 bg-muted rounded">
                <div className="text-lg font-semibold">
                  {stats.item_count}
                </div>
                <div className="text-muted-foreground text-xs">Items</div>
              </div>
              <div className="text-center p-2 bg-muted rounded">
                <div className="text-lg font-semibold">
                  {stats.subscriber_count}
                </div>
                <div className="text-muted-foreground text-xs">Subscribers</div>
              </div>
              <div className="text-center p-2 bg-muted rounded">
                <div className="text-lg font-semibold">
                  {stats.contributor_count}
                </div>
                <div className="text-muted-foreground text-xs">Contributors</div>
              </div>
            </div>

            {Object.keys(stats.entity_type_counts).length > 0 && (
              <div className="text-sm space-y-1">
                <p className="text-muted-foreground text-xs">
                  Entity type breakdown:
                </p>
                {Object.entries(stats.entity_type_counts)
                  .sort(([, a], [, b]) => (b as number) - (a as number))
                  .map(([type, count]) => (
                    <div key={type} className="flex justify-between items-center">
                      <EntityTypeBadge type={type} />
                      <span className="text-muted-foreground">{count}</span>
                    </div>
                  ))}
              </div>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No stats available</p>
        )}
      </div>

      {/* Items list */}
      <div>
        <h4 className="text-sm font-medium mb-2">Items</h4>
        {detailLoading ? (
          <p className="text-sm text-muted-foreground">Loading items...</p>
        ) : detail?.items && detail.items.length > 0 ? (
          <div className="border border-border rounded overflow-hidden">
            <table className="w-full text-xs">
              <thead className="bg-muted/50">
                <tr>
                  <th className="text-left p-2 font-medium">#</th>
                  <th className="text-left p-2 font-medium">Name</th>
                  <th className="text-left p-2 font-medium">Type</th>
                  <th className="text-left p-2 font-medium">Added By</th>
                  <th className="text-left p-2 font-medium">Added</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.items.map((item) => (
                  <tr key={item.id}>
                    <td className="p-2 text-muted-foreground">
                      {item.position + 1}
                    </td>
                    <td className="p-2">
                      <span className="font-medium">{item.entity_name}</span>
                      {item.notes && (
                        <span
                          className="ml-1 text-muted-foreground"
                          title={item.notes}
                        >
                          (note)
                        </span>
                      )}
                    </td>
                    <td className="p-2">
                      <EntityTypeBadge type={item.entity_type} />
                    </td>
                    <td className="p-2 text-muted-foreground">
                      {item.added_by_name}
                    </td>
                    <td className="p-2 text-muted-foreground">
                      {new Date(item.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No items in this collection</p>
        )}
      </div>

      {/* Actions */}
      <div className="flex gap-2 pt-2 border-t border-border">
        <button
          onClick={handleDelete}
          disabled={deleteCollection.isPending}
          className="px-3 py-1 bg-red-500/20 text-red-400 rounded text-sm hover:bg-red-500/30 disabled:opacity-50"
        >
          {deleteCollection.isPending ? 'Deleting...' : 'Delete Collection'}
        </button>
      </div>

      {deleteCollection.error && (
        <p className="text-sm text-red-400">
          {deleteCollection.error instanceof Error
            ? deleteCollection.error.message
            : 'Delete failed'}
        </p>
      )}
    </div>
  )
}

export function CollectionManagement() {
  const { data, isLoading, error } = useCollections()
  const setFeatured = useSetFeatured()
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null)
  // PSY-609: surface featured-toggle failures so admins aren't left
  // wondering why the switch flipped back. Mirrors the LabelManagement
  // setError pattern called out in the audit.
  const [featuredError, setFeaturedError] = useState<string | null>(null)

  if (isLoading)
    return <p className="text-muted-foreground">Loading collections...</p>
  if (error) return <p className="text-red-400">Failed to load collections</p>

  const collections = data?.collections ?? []
  const selectedCollection = collections.find((c) => c.slug === selectedSlug)

  const columns: AdminTableColumn<Collection>[] = [
    {
      key: 'title',
      header: 'Title',
      render: (c) => (
        <>
          <div className="font-medium">{c.title}</div>
          <div className="text-xs text-muted-foreground">/{c.slug}</div>
        </>
      ),
    },
    {
      key: 'creator',
      header: 'Creator',
      cellClassName: 'text-muted-foreground',
      render: (c) => c.creator_name,
    },
    { key: 'items', header: 'Items', align: 'center', render: (c) => c.item_count },
    {
      key: 'subscribers',
      header: 'Subscribers',
      align: 'center',
      render: (c) => c.subscriber_count,
    },
    {
      key: 'featured',
      header: 'Featured',
      align: 'center',
      stopRowClick: true,
      render: (c) => {
        // PSY-1504: "featured since {date}" beside the toggle for currently-
        // featured rows. Fed by the PSY-1500 list payload (featured_at +
        // featured_at_estimated) — no per-row fetch. The label refreshes with
        // the list on toggle (useSetFeatured invalidates collections.all).
        const sinceLabel = c.is_featured
          ? featuredSinceLabel(c.featured_at, c.featured_at_estimated)
          : null
        return (
          <div className="flex flex-col items-center gap-1">
            <Switch
              checked={c.is_featured}
              onCheckedChange={(checked) => {
                setFeaturedError(null)
                setFeatured.mutate(
                  { slug: c.slug, featured: checked },
                  {
                    // Clears-on-next-success per the sticky-on-error
                    // mutation-feedback convention (PSY-609).
                    onSuccess: () => setFeaturedError(null),
                    onError: (err) =>
                      setFeaturedError(
                        err instanceof Error
                          ? err.message
                          : 'Failed to update featured status'
                      ),
                  }
                )
              }}
              disabled={setFeatured.isPending}
              size="sm"
            />
            {sinceLabel && (
              <span className="whitespace-nowrap text-[11px] leading-tight text-muted-foreground">
                {sinceLabel}
              </span>
            )}
          </div>
        )
      },
    },
    {
      key: 'public',
      header: 'Public',
      align: 'center',
      render: (c) =>
        c.is_public ? (
          <span className="text-green-400 text-xs">Yes</span>
        ) : (
          <span className="text-muted-foreground text-xs">No</span>
        ),
    },
    {
      key: 'created',
      header: 'Created',
      cellClassName: 'text-muted-foreground text-xs',
      render: (c) => new Date(c.created_at).toLocaleDateString(),
    },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Collections</h2>
        <span className="text-sm text-muted-foreground">
          {data?.total ?? 0} total
        </span>
      </div>

      {/* PSY-609: featured-toggle error banner. Sticky until the next
          successful toggle clears it (handled in the Switch onCheckedChange). */}
      {featuredError && (
        <div
          role="alert"
          data-testid="featured-toggle-error"
          className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
        >
          {featuredError}
        </div>
      )}

      {collections.length === 0 ? (
        <p className="text-muted-foreground">No collections yet</p>
      ) : (
        <>
          {/* Collections table (AdminTable — PSY-910) */}
          <AdminTable
            columns={columns}
            rows={collections}
            rowKey={(c) => c.id}
            onRowClick={(c) =>
              setSelectedSlug(selectedSlug === c.slug ? null : c.slug)
            }
            rowLabel={(c) => `Collection: ${c.title}`}
            rowClassName={(c) =>
              selectedSlug === c.slug ? 'bg-muted/50' : undefined
            }
          />

          {/* Detail panel */}
          {selectedCollection && (
            <CollectionDetailPanel
              collection={selectedCollection}
              onClose={() => setSelectedSlug(null)}
            />
          )}
        </>
      )}
    </div>
  )
}
