'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ChevronDown,
  ChevronRight,
  Hash,
  Loader2,
  Pencil,
  Search,
  Network,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { useSearchTags } from '../hooks'
import { useGenreHierarchy, useSetTagParent } from './useAdminTags'
import type { GenreHierarchyNode, GenreHierarchyTag, TagListItem } from '../types'

// ──────────────────────────────────────────────
// Tree assembly
// ──────────────────────────────────────────────

/**
 * Build a forest of GenreHierarchyNodes from the backend flat list.
 * Orphaned nodes (parent_id points to a tag not in the list — shouldn't
 * happen but guard anyway) are promoted to roots.
 */
export function buildHierarchyTree(tags: GenreHierarchyTag[]): GenreHierarchyNode[] {
  const byId = new Map<number, GenreHierarchyNode>()
  for (const t of tags) {
    byId.set(t.id, { ...t, depth: 0, children: [] })
  }
  const roots: GenreHierarchyNode[] = []
  for (const node of byId.values()) {
    if (node.parent_id != null && byId.has(node.parent_id)) {
      const parent = byId.get(node.parent_id)!
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }
  // Stamp depth by BFS so children of deeply-nested roots still indent right.
  const assignDepth = (node: GenreHierarchyNode, depth: number) => {
    node.depth = depth
    node.children.sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    )
    for (const c of node.children) assignDepth(c, depth + 1)
  }
  roots.sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  )
  for (const r of roots) assignDepth(r, 0)
  return roots
}

/**
 * Collect the IDs of a node and every descendant — used to exclude a tag's
 * subtree from the parent picker (you can't parent a tag to one of its own
 * descendants). Backend rejects this too; the pre-filter is UX polish.
 */
function collectSubtreeIds(node: GenreHierarchyNode, acc: Set<number>) {
  acc.add(node.id)
  for (const c of node.children) collectSubtreeIds(c, acc)
}

function findNodeById(
  nodes: GenreHierarchyNode[],
  id: number
): GenreHierarchyNode | null {
  for (const n of nodes) {
    if (n.id === id) return n
    const found = findNodeById(n.children, id)
    if (found) return found
  }
  return null
}

// ──────────────────────────────────────────────
// Parent picker dialog
// ──────────────────────────────────────────────

interface ParentPickerDialogProps {
  open: boolean
  tagId: number | null
  tagName: string
  currentParentId: number | null
  /** IDs to exclude from candidates — the tag itself and its descendants. */
  excludedIds: Set<number>
  onClose: () => void
}

function ParentPickerDialog({
  open,
  tagId,
  tagName,
  currentParentId,
  excludedIds,
  onClose,
}: ParentPickerDialogProps) {
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  const [selected, setSelected] = useState<TagListItem | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [wantsClear, setWantsClear] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 200)
    return () => clearTimeout(t)
  }, [search])

  useEffect(() => {
    if (!open) return
    setSearch('')
    setDebounced('')
    setSelected(null)
    setError(null)
    setWantsClear(false)
  }, [open, tagId])

  // Genre-only search — hierarchy is genre-only end-to-end.
  const { data: searchData, isLoading: searching } = useSearchTags(
    debounced,
    15,
    'genre'
  )

  const candidates = useMemo(() => {
    const results = searchData?.tags ?? []
    return results.filter(t => !excludedIds.has(t.id))
  }, [searchData, excludedIds])

  const setParent = useSetTagParent()

  const handleConfirm = useCallback(() => {
    if (!tagId) return
    setError(null)
    const parentId = wantsClear ? null : selected?.id ?? null
    if (!wantsClear && parentId === currentParentId) {
      // No-op; just close to avoid a pointless network round-trip.
      onClose()
      return
    }
    setParent.mutate(
      { tagId, parentId },
      {
        onSuccess: () => onClose(),
        onError: err => {
          setError(err instanceof Error ? err.message : 'Failed to set parent')
        },
      }
    )
  }, [tagId, wantsClear, selected, currentParentId, setParent, onClose])

  const canConfirm = wantsClear
    ? currentParentId !== null
    : selected !== null && selected.id !== currentParentId

  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()}>
      <DialogContent className="max-w-md max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Set parent for &quot;{tagName}&quot;</DialogTitle>
          <DialogDescription>
            Search for a genre tag to use as the new parent, or clear the
            parent to make this tag a root.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {error && (
            <div
              className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
              role="alert"
            >
              {error}
            </div>
          )}

          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search genre tags..."
              value={search}
              onChange={e => {
                setSearch(e.target.value)
                setWantsClear(false)
              }}
              className="pl-9"
              autoFocus
              aria-label="Search for a parent tag"
            />
          </div>

          {searching && debounced.length >= 2 && (
            <div className="flex items-center justify-center py-3">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}

          {!searching && debounced.length >= 2 && candidates.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No genre tags match that search (excluding this tag and its
              descendants).
            </p>
          )}

          {candidates.length > 0 && (
            <ul
              className="max-h-60 overflow-y-auto rounded-md border divide-y"
              aria-label="Candidate parent tags"
            >
              {candidates.map(c => (
                <li key={c.id}>
                  <button
                    type="button"
                    onClick={() => {
                      setSelected(c)
                      setWantsClear(false)
                    }}
                    className={cn(
                      'flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-muted/50 transition-colors',
                      selected?.id === c.id && 'bg-muted'
                    )}
                  >
                    <Hash className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="flex-1 truncate">{c.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {c.usage_count} uses
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}

          {selected && !wantsClear && (
            <p className="text-sm">
              Selected parent:{' '}
              <span className="font-medium">{selected.name}</span>
            </p>
          )}

          <div className="border-t pt-3">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={wantsClear}
                onChange={e => {
                  setWantsClear(e.target.checked)
                  if (e.target.checked) setSelected(null)
                }}
                className="h-4 w-4 rounded border-muted-foreground"
              />
              <span>
                Clear parent (make this tag a root)
              </span>
            </label>
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={setParent.isPending}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={handleConfirm}
            disabled={!canConfirm || setParent.isPending}
          >
            {setParent.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              'Save'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ──────────────────────────────────────────────
// Tree node row
// ──────────────────────────────────────────────

interface NodeRowProps {
  node: GenreHierarchyNode
  expandedIds: Set<number>
  onToggle: (id: number) => void
  onEdit: (node: GenreHierarchyNode) => void
}

function NodeRow({ node, expandedIds, onToggle, onEdit }: NodeRowProps) {
  const expanded = expandedIds.has(node.id)
  const hasChildren = node.children.length > 0

  return (
    <>
      <li
        className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50"
        // Indentation via inline style keeps this flexible for arbitrary depths
        // without generating a Tailwind class per level.
        style={{ paddingLeft: `${node.depth * 20 + 8}px` }}
        data-testid="hierarchy-row"
        data-tag-id={node.id}
      >
        <button
          type="button"
          onClick={() => hasChildren && onToggle(node.id)}
          disabled={!hasChildren}
          aria-label={
            hasChildren
              ? expanded
                ? `Collapse children of ${node.name}`
                : `Expand children of ${node.name}`
              : undefined
          }
          className={cn(
            'flex h-5 w-5 shrink-0 items-center justify-center rounded',
            hasChildren
              ? 'hover:bg-muted text-muted-foreground'
              : 'text-transparent cursor-default'
          )}
        >
          {hasChildren ? (
            expanded ? (
              <ChevronDown className="h-3.5 w-3.5" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" />
            )
          ) : (
            // Reserve the slot for alignment.
            <span className="h-3.5 w-3.5" />
          )}
        </button>

        <Hash className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="flex-1 truncate text-sm font-medium">{node.name}</span>
        {node.is_official && (
          <Badge variant="outline" className="text-[10px]">
            official
          </Badge>
        )}
        <span className="text-xs text-muted-foreground tabular-nums">
          {node.usage_count}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 w-7 p-0"
          onClick={() => onEdit(node)}
          aria-label={`Edit parent of ${node.name}`}
        >
          <Pencil className="h-3.5 w-3.5" />
        </Button>
      </li>

      {expanded &&
        node.children.map(child => (
          <NodeRow
            key={child.id}
            node={child}
            expandedIds={expandedIds}
            onToggle={onToggle}
            onEdit={onEdit}
          />
        ))}
    </>
  )
}

// ──────────────────────────────────────────────
// Main editor
// ──────────────────────────────────────────────

export function TagHierarchyEditor() {
  const { data, isLoading, error } = useGenreHierarchy()
  const [search, setSearch] = useState('')
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set())
  const [editingTag, setEditingTag] = useState<GenreHierarchyNode | null>(null)

  const allTags = useMemo(() => data?.tags ?? [], [data])
  const roots = useMemo(() => buildHierarchyTree(allTags), [allTags])

  // Filtered view: when a search is active, flatten to matching tags so
  // admins can jump to a tag buried deep in the tree. Matches are shown
  // without their tree context — tradeoff for simplicity.
  const filteredRoots = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return roots
    const matches: GenreHierarchyNode[] = []
    const visit = (node: GenreHierarchyNode) => {
      if (node.name.toLowerCase().includes(q)) {
        // Flatten matches to depth 0 so the list stays readable during search.
        matches.push({ ...node, depth: 0, children: [] })
      }
      for (const c of node.children) visit(c)
    }
    for (const r of roots) visit(r)
    return matches
  }, [search, roots])

  const handleToggle = useCallback((id: number) => {
    setExpandedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  // Auto-expand roots on first load so admins see the structure immediately.
  useEffect(() => {
    if (roots.length > 0 && expandedIds.size === 0) {
      setExpandedIds(new Set(roots.map(r => r.id)))
    }
    // Intentionally depends only on roots.length; don't re-seed on every
    // re-render that preserves the same tree.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roots.length])

  const excludedIds = useMemo(() => {
    if (!editingTag) return new Set<number>()
    const fullNode = findNodeById(roots, editingTag.id)
    const acc = new Set<number>()
    if (fullNode) collectSubtreeIds(fullNode, acc)
    else acc.add(editingTag.id)
    return acc
  }, [editingTag, roots])

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Network className="h-5 w-5" />
          Genre Hierarchy
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Set parent/child relationships for genre tags (e.g.{' '}
          <span className="font-mono">shoegaze</span> under{' '}
          <span className="font-mono">post-punk</span>). Non-genre categories
          are flat and do not participate in the hierarchy.
        </p>
      </div>

      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Filter genre tags..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="pl-9 pr-8"
          aria-label="Filter genre tags"
        />
        {search && (
          <button
            type="button"
            onClick={() => setSearch('')}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            aria-label="Clear filter"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div
          className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center"
          role="alert"
        >
          <p className="text-destructive">
            {error instanceof Error
              ? error.message
              : 'Failed to load genre hierarchy.'}
          </p>
        </div>
      )}

      {!isLoading && !error && allTags.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Hash className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No Genre Tags</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            Create genre tags in the Tags tab to start building a hierarchy.
          </p>
        </div>
      )}

      {!isLoading && !error && allTags.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {allTags.length} genre tag{allTags.length !== 1 ? 's' : ''}
            {search &&
              ` — ${filteredRoots.length} match${filteredRoots.length !== 1 ? 'es' : ''}`}
          </div>

          {filteredRoots.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4">
              No tags match &quot;{search}&quot;.
            </p>
          ) : (
            <ul
              className="space-y-0.5 rounded-lg border p-1"
              data-testid="hierarchy-tree"
            >
              {filteredRoots.map(node => (
                <NodeRow
                  key={node.id}
                  node={node}
                  expandedIds={expandedIds}
                  onToggle={handleToggle}
                  onEdit={setEditingTag}
                />
              ))}
            </ul>
          )}
        </>
      )}

      <ParentPickerDialog
        open={editingTag !== null}
        tagId={editingTag?.id ?? null}
        tagName={editingTag?.name ?? ''}
        currentParentId={editingTag?.parent_id ?? null}
        excludedIds={excludedIds}
        onClose={() => setEditingTag(null)}
      />
    </div>
  )
}

export default TagHierarchyEditor
