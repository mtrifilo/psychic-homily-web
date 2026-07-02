'use client'

/**
 * ClusterLegend (PSY-1305)
 *
 * Shared cluster-toggle pills for graph sections — extracted from the
 * identical copies in SceneGraph and StationGraph. Lives beside EdgeLegend
 * (which covers edge TYPES; this covers node clusters).
 *
 * Encodes the PSY-1083 theme-token contract: `clusterColorCSS` returns a
 * `var(--chart-N)` expression, so the pills track theme changes with no JS.
 * Callers own the toggle STATE (the hidden-ID set); this renders + reports.
 */

import { Eye, EyeOff } from 'lucide-react'
import { clusterColorCSS } from './graphPalette'
import type { GraphCluster } from './ForceGraphView'

interface ClusterLegendProps {
  clusters: GraphCluster[]
  hiddenClusterIDs: Set<string>
  onToggle: (clusterID: string) => void
}

export function ClusterLegend({ clusters, hiddenClusterIDs, onToggle }: ClusterLegendProps) {
  if (clusters.length === 0) return null

  return (
    <div className="flex flex-wrap gap-1.5">
      {clusters.map(cluster => {
        const hidden = hiddenClusterIDs.has(cluster.id)
        return (
          <button
            key={cluster.id}
            onClick={() => onToggle(cluster.id)}
            aria-pressed={!hidden}
            className={`inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full border transition-opacity ${
              hidden ? 'opacity-40' : 'opacity-100'
            }`}
            style={{
              borderColor: clusterColorCSS(cluster.color_index),
              color: clusterColorCSS(cluster.color_index),
            }}
            title={hidden ? `Show ${cluster.label}` : `Hide ${cluster.label}`}
          >
            <span
              className="inline-block w-2 h-2 rounded-full"
              style={{ backgroundColor: clusterColorCSS(cluster.color_index) }}
            />
            <span className="text-foreground/85">
              {cluster.label} ({cluster.size})
            </span>
            {hidden ? (
              <EyeOff className="h-3 w-3" aria-hidden="true" />
            ) : (
              <Eye className="h-3 w-3" aria-hidden="true" />
            )}
          </button>
        )
      })}
    </div>
  )
}
