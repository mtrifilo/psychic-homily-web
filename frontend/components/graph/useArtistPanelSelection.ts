'use client'

/**
 * useArtistPanelSelection — the shared node-select → ArtistContextPanel
 * wiring for Section-class graph surfaces (rule of three: HomeSceneGraph,
 * SceneGraphVisualization, StationGraphVisualization).
 *
 * Locked grammar: on Section-class surfaces a node click SELECTS into the
 * shared ArtistContextPanel; navigation happens only via the panel's
 * "Open page →" (the canvas aria-label advertises this via
 * `graphSelectGestureHint`). This hook owns the conventions so they can't
 * drift per surface:
 *   - a second click on the selected node deselects ("put it away");
 *   - background click closes only while a panel is open — a plain click on
 *     empty canvas must not steal document focus (and possibly scroll a
 *     partially-visible canvas into view) as the side effect of a no-op;
 *   - Esc closes via the panel's DismissableLayer → onClose →
 *     `handlePanelClose` (Radix preventDefaults in the capture phase, so an
 *     enclosing fullscreen overlay's own Esc listener — which skips
 *     defaultPrevented — closes only on the NEXT press);
 *   - focus returns to the canvas wrap on close ONLY when it would otherwise
 *     be orphaned: on the unmounting panel, or already lost to document.body
 *     (the PSY-1313 lesson — after the panel unmounts, focus inside it drops
 *     to body). An Esc pressed while focus sits on the accessible node list
 *     must NOT yank focus off that list (deferred PR #1562 finding), which
 *     is why the caller passes `panelRef` to ArtistContextPanel — the check
 *     needs the panel element to tell panel content from other wrap content;
 *   - an edge click opening the ConnectionPanel deselects the node panel so
 *     the two inspectors never stack (mirrors ForceGraphView's own symmetry:
 *     node click closes the inspector). No focus move — the user's attention
 *     just shifted to the connection inspector.
 *
 * The selection is resolved against the CURRENT payload on every render via
 * `resolveNode` — a legend hide or a refetch that drops the node puts the
 * panel away rather than strand it naming an off-canvas artist. Stale state
 * clears via React's adjust-state-during-render idiom, not a
 * setState-in-effect (react-hooks/set-state-in-effect + a cascading render).
 *
 * The caller owns: the canvas wrap `<div ref={canvasWrapRef} tabIndex={-1}>`
 * hosting both the canvas and the floated panel, the card fetch
 * (useArtistGraphCard), panel positioning, and any extra per-surface click
 * side effects (wrap `handleNodeClick`, e.g. HomeSceneGraph's scene pin).
 */

import { useCallback, useRef, useState } from 'react'
import type { RefObject } from 'react'
// Type-only import, deliberately: HomeSceneGraph consumes this hook in its
// statically-mounted section while loading ForceGraphView in its own
// dynamic(ssr:false) chunk (PSY-868) — a VALUE import here would drag the
// whole canvas module into the homepage's initial JS. The value-level
// companion (resolveNodeInVisibleClusters) lives in its own module for the
// same reason.
import type { GraphNode } from './ForceGraphView'

export interface UseArtistPanelSelectionOptions<TNode extends GraphNode> {
  /**
   * Resolve a previously selected node against the current payload/filters.
   * Return null when the node is no longer on canvas — the selection clears
   * (during render) and the panel unmounts.
   */
  resolveNode: (selected: GraphNode) => TNode | null
}

export interface UseArtistPanelSelectionResult<TNode extends GraphNode> {
  /** The selection resolved against the current payload; null = no panel. */
  selectedNode: TNode | null
  /** Attach to the relative-positioned wrap around canvas + panel. */
  canvasWrapRef: RefObject<HTMLDivElement | null>
  /** Pass to ArtistContextPanel's `panelRef` — the focus-return check needs it. */
  panelRef: RefObject<HTMLElement | null>
  /** ForceGraphView `onNodeClick`: select, or deselect on the same node. */
  handleNodeClick: (node: GraphNode) => void
  /** ForceGraphView `onBackgroundClick`: close, only while a panel is open. */
  handleBackgroundClick: () => void
  /** ArtistContextPanel `onClose` (X button + Esc): close + focus return. */
  handlePanelClose: () => void
  /** ForceGraphView `onConnectionInspectOpen`: deselect so panels never stack. */
  handleConnectionInspectOpen: () => void
  /** Imperative clear with no focus side effect (e.g. scene rotation). */
  clearSelection: () => void
}

export function useArtistPanelSelection<TNode extends GraphNode>({
  resolveNode,
}: UseArtistPanelSelectionOptions<TNode>): UseArtistPanelSelectionResult<TNode> {
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null)
  const canvasWrapRef = useRef<HTMLDivElement>(null)
  const panelRef = useRef<HTMLElement>(null)

  const currentSelectedNode = selectedNode ? resolveNode(selectedNode) : null
  if (selectedNode && !currentSelectedNode) {
    setSelectedNode(null)
  }

  const handleNodeClick = useCallback((node: GraphNode) => {
    setSelectedNode(prev => (prev?.id === node.id ? null : node))
  }, [])

  const handlePanelClose = useCallback(() => {
    setSelectedNode(null)
    // Focus return, guarded (see the header comment): only when focus is
    // being orphaned by the panel unmount or was already lost to body.
    const active = document.activeElement
    const orphaned =
      !(active instanceof HTMLElement) ||
      active === document.body ||
      Boolean(panelRef.current?.contains(active))
    if (orphaned) canvasWrapRef.current?.focus()
  }, [])

  const handleBackgroundClick = useCallback(() => {
    if (selectedNode) handlePanelClose()
  }, [selectedNode, handlePanelClose])

  const clearSelection = useCallback(() => {
    setSelectedNode(null)
  }, [])

  // Same plain clear, exposed under its wiring-intent name — keep them one
  // implementation so the two can't silently diverge.
  const handleConnectionInspectOpen = clearSelection

  return {
    selectedNode: currentSelectedNode,
    canvasWrapRef,
    panelRef,
    handleNodeClick,
    handleBackgroundClick,
    handlePanelClose,
    handleConnectionInspectOpen,
    clearSelection,
  }
}
