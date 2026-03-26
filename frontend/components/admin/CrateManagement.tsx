'use client'

import { useState } from 'react'
import {
  useCrates,
  useCrate,
  useCrateStats,
  useSetFeatured,
  useDeleteCrate,
} from '@/features/crates'
import type { Crate, CrateDetail, CrateStats } from '@/features/crates'
import { Switch } from '@/components/ui/switch'

function EntityTypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    artist: 'bg-purple-500/20 text-purple-400',
    release: 'bg-blue-500/20 text-blue-400',
    label: 'bg-green-500/20 text-green-400',
    show: 'bg-amber-500/20 text-amber-400',
    venue: 'bg-rose-500/20 text-rose-400',
    festival: 'bg-cyan-500/20 text-cyan-400',
  }

  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${colors[type] || 'bg-muted text-muted-foreground'}`}
    >
      {type}
    </span>
  )
}

function CrateDetailPanel({
  crate,
  onClose,
}: {
  crate: Crate
  onClose: () => void
}) {
  const { data: detail, isLoading: detailLoading } = useCrate(crate.slug)
  const { data: stats, isLoading: statsLoading } = useCrateStats(crate.slug)
  const deleteCrate = useDeleteCrate()

  const handleDelete = () => {
    if (
      window.confirm(
        `Delete crate "${crate.title}"? This cannot be undone.`
      )
    ) {
      deleteCrate.mutate(
        { slug: crate.slug },
        { onSuccess: () => onClose() }
      )
    }
  }

  return (
    <div className="border border-border rounded-lg p-4 space-y-4 bg-card">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">{crate.title}</h3>
          <p className="text-sm text-muted-foreground">
            /{crate.slug} &middot; by {crate.creator_name}
          </p>
        </div>
        <button
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground text-sm"
        >
          Close
        </button>
      </div>

      {crate.description && (
        <p className="text-sm text-muted-foreground">{crate.description}</p>
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
          <p className="text-sm text-muted-foreground">No items in this crate</p>
        )}
      </div>

      {/* Actions */}
      <div className="flex gap-2 pt-2 border-t border-border">
        <button
          onClick={handleDelete}
          disabled={deleteCrate.isPending}
          className="px-3 py-1 bg-red-500/20 text-red-400 rounded text-sm hover:bg-red-500/30 disabled:opacity-50"
        >
          {deleteCrate.isPending ? 'Deleting...' : 'Delete Crate'}
        </button>
      </div>

      {deleteCrate.error && (
        <p className="text-sm text-red-400">
          {deleteCrate.error instanceof Error
            ? deleteCrate.error.message
            : 'Delete failed'}
        </p>
      )}
    </div>
  )
}

export function CrateManagement() {
  const { data, isLoading, error } = useCrates()
  const setFeatured = useSetFeatured()
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null)

  if (isLoading)
    return <p className="text-muted-foreground">Loading crates...</p>
  if (error) return <p className="text-red-400">Failed to load crates</p>

  const crates = data?.crates ?? []
  const selectedCrate = crates.find((c) => c.slug === selectedSlug)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Crates</h2>
        <span className="text-sm text-muted-foreground">
          {data?.total ?? 0} total
        </span>
      </div>

      {crates.length === 0 ? (
        <p className="text-muted-foreground">No crates yet</p>
      ) : (
        <>
          {/* Crates table */}
          <div className="border border-border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-muted/50">
                <tr>
                  <th className="text-left p-3 font-medium">Title</th>
                  <th className="text-left p-3 font-medium">Creator</th>
                  <th className="text-center p-3 font-medium">Items</th>
                  <th className="text-center p-3 font-medium">Subscribers</th>
                  <th className="text-center p-3 font-medium">Featured</th>
                  <th className="text-center p-3 font-medium">Public</th>
                  <th className="text-left p-3 font-medium">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {crates.map((crate) => (
                  <tr
                    key={crate.id}
                    onClick={() =>
                      setSelectedSlug(
                        selectedSlug === crate.slug
                          ? null
                          : crate.slug
                      )
                    }
                    className={`cursor-pointer hover:bg-muted/30 ${
                      selectedSlug === crate.slug ? 'bg-muted/50' : ''
                    }`}
                  >
                    <td className="p-3">
                      <div className="font-medium">{crate.title}</div>
                      <div className="text-xs text-muted-foreground">
                        /{crate.slug}
                      </div>
                    </td>
                    <td className="p-3 text-muted-foreground">
                      {crate.creator_name}
                    </td>
                    <td className="p-3 text-center">{crate.item_count}</td>
                    <td className="p-3 text-center">
                      {crate.subscriber_count}
                    </td>
                    <td
                      className="p-3 text-center"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Switch
                        checked={crate.is_featured}
                        onCheckedChange={(checked) => {
                          setFeatured.mutate({
                            slug: crate.slug,
                            featured: checked,
                          })
                        }}
                        disabled={setFeatured.isPending}
                        size="sm"
                      />
                    </td>
                    <td className="p-3 text-center">
                      {crate.is_public ? (
                        <span className="text-green-400 text-xs">Yes</span>
                      ) : (
                        <span className="text-muted-foreground text-xs">No</span>
                      )}
                    </td>
                    <td className="p-3 text-muted-foreground text-xs">
                      {new Date(crate.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Detail panel */}
          {selectedCrate && (
            <CrateDetailPanel
              crate={selectedCrate}
              onClose={() => setSelectedSlug(null)}
            />
          )}
        </>
      )}
    </div>
  )
}
