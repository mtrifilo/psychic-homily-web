'use client'

/**
 * GraphAccessibleTree (PSY-1304) — the keyboard-navigable, screen-reader-
 * accessible equivalent of the `role="img"` graph canvas.
 *
 * The canvas is excluded from tab order; this `role="tree"` renders the same
 * connections and drives the SAME expand-on-demand traversal a canvas
 * node-click does. It is the "parallel structured representation" the
 * accessible-charts research (Data Navigator; Deque) prescribes.
 *
 * Interaction (WAI-ARIA tree pattern, roving tabindex — one item in the tab
 * order at a time):
 *   - ↑ / ↓        move between visible rows
 *   - → (collapsed) expand this node's connections (mirrors a canvas node-click)
 *   - → (expanded)  move to the first child
 *   - ← (expanded)  collapse; ← (collapsed) move to the parent
 *   - Enter / Space toggle expand/collapse
 *   - Home / End    first / last visible row
 *
 * Every artist can be expanded (its ego is fetched on demand), so every item
 * carries aria-expanded; an expanded node that revealed nothing new is still
 * `aria-expanded=true` with no children. The tree is FLAT in the DOM (no
 * nested `role=group`) and conveys depth via aria-level/setsize/posinset —
 * a valid ARIA tree shape that keeps focus management simple.
 *
 * Shared (components/graph/) so the ego dialog, and later the page sidebar +
 * scene/station graphs, mount one implementation.
 */

import { useCallback, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import type { AccessibleTreeGraphNode, AccessibleTreeRow } from './graphTreeModel'

export interface GraphAccessibleTreeProps<N extends AccessibleTreeGraphNode> {
  /** Rendered on the `role="tree"` element so a canvas can aria-describedby it. */
  id?: string
  /** Flattened visible rows (see flattenVisibleTree). */
  rows: AccessibleTreeRow<N>[]
  /** Accessible name for the tree, e.g. "Connections for Faetooth". */
  label: string
  /** Expand (collapsed) or collapse (expanded) the node's connections. */
  onToggleExpand: (node: N) => void
  /** Optional trailing metadata per row (location, edge summary, …). */
  renderRowMeta?: (node: N) => ReactNode
  className?: string
  /** Message shown when there are no connections to navigate. */
  emptyLabel?: string
}

export function GraphAccessibleTree<N extends AccessibleTreeGraphNode>({
  id,
  rows,
  label,
  onToggleExpand,
  renderRowMeta,
  className,
  emptyLabel = 'No connections to navigate.',
}: GraphAccessibleTreeProps<N>) {
  const itemRefs = useRef(new Map<number, HTMLLIElement>())
  const [focusedId, setFocusedId] = useState<number | null>(rows[0]?.node.id ?? null)

  // Keep the roving tabstop on a still-visible row as rows change (expand
  // reveals children, collapse removes them). Derived DURING RENDER, not in an
  // effect — the repo's react-hooks lint errors on setState-in-effect, and this
  // is React's recommended "adjust state during render" idiom for a derived
  // reset. It never grabs focus, so a data refresh can't yank focus into the tree.
  const focusedVisible = focusedId !== null && rows.some(r => r.node.id === focusedId)
  const effectiveFocusedId = rows.length === 0 ? null : focusedVisible ? focusedId : rows[0].node.id
  if (effectiveFocusedId !== focusedId) {
    setFocusedId(effectiveFocusedId)
  }

  const focusRow = useCallback((nodeId: number) => {
    setFocusedId(nodeId)
    itemRefs.current.get(nodeId)?.focus()
  }, [])

  const onKeyDown = useCallback(
    (e: KeyboardEvent<HTMLUListElement>) => {
      if (effectiveFocusedId === null) return
      const idx = rows.findIndex(r => r.node.id === effectiveFocusedId)
      if (idx === -1) return
      const row = rows[idx]
      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault()
          if (idx < rows.length - 1) focusRow(rows[idx + 1].node.id)
          break
        case 'ArrowUp':
          e.preventDefault()
          if (idx > 0) focusRow(rows[idx - 1].node.id)
          break
        case 'Home':
          e.preventDefault()
          focusRow(rows[0].node.id)
          break
        case 'End':
          e.preventDefault()
          focusRow(rows[rows.length - 1].node.id)
          break
        case 'ArrowRight':
          e.preventDefault()
          if (!row.expanded) onToggleExpand(row.node)
          else if (idx < rows.length - 1 && rows[idx + 1].level > row.level) focusRow(rows[idx + 1].node.id)
          break
        case 'ArrowLeft':
          e.preventDefault()
          if (row.expanded) onToggleExpand(row.node)
          else {
            for (let i = idx - 1; i >= 0; i--) {
              if (rows[i].level < row.level) {
                focusRow(rows[i].node.id)
                break
              }
            }
          }
          break
        case 'Enter':
        case ' ':
          e.preventDefault()
          onToggleExpand(row.node)
          break
      }
    },
    [rows, effectiveFocusedId, focusRow, onToggleExpand],
  )

  if (rows.length === 0) {
    return (
      <p id={id} className={cn('text-xs text-muted-foreground', className)}>
        {emptyLabel}
      </p>
    )
  }

  return (
    <ul
      role="tree"
      id={id}
      aria-label={label}
      className={cn('text-sm', className)}
      onKeyDown={onKeyDown}
    >
      {rows.map(row => {
        const focused = row.node.id === effectiveFocusedId
        return (
          <li
            key={row.node.id}
            role="treeitem"
            ref={el => {
              if (el) itemRefs.current.set(row.node.id, el)
              else itemRefs.current.delete(row.node.id)
            }}
            tabIndex={focused ? 0 : -1}
            // Single-select nav tree: the roving-focused row is the selected one.
            aria-selected={focused}
            aria-expanded={row.expanded}
            aria-level={row.level}
            aria-setsize={row.setSize}
            aria-posinset={row.posInSet}
            aria-busy={row.expanding || undefined}
            onClick={() => {
              focusRow(row.node.id)
              onToggleExpand(row.node)
            }}
            style={{ paddingLeft: `${(row.level - 1) * 16 + 8}px` }}
            className={cn(
              'flex items-center gap-2 rounded-sm py-1 pr-2 cursor-pointer outline-none',
              'hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring',
              focused && 'bg-accent',
            )}
          >
            <span aria-hidden="true" className="w-3 shrink-0 text-muted-foreground">
              {row.expanding ? '⋯' : row.expanded ? '▾' : '▸'}
            </span>
            <span className="min-w-0 flex-1 truncate text-foreground">{row.node.name}</span>
            {renderRowMeta && (
              <span className="shrink-0 text-xs text-muted-foreground">{renderRowMeta(row.node)}</span>
            )}
          </li>
        )
      })}
    </ul>
  )
}
