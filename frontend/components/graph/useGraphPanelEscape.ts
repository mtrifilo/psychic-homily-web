'use client'

/**
 * useGraphPanelEscape (PSY-1360) — the coordinated capture-phase Escape dismiss
 * for the graph inspector panels (ArtistContextPanel + ConnectionPanel).
 *
 * NOT a general-purpose "capture Escape" utility: every caller joins ONE shared
 * module-level LIFO stack (escapeStack), so the hook only makes sense for panels
 * that must dismiss innermost-first relative to EACH OTHER. Keep its use to the
 * graph panels — a toast/dropdown/dialog elsewhere reaching for it would silently
 * interleave with the graph panels' stack. That coupling is the whole point of
 * the shared stack; the name says "GraphPanel" so it isn't mistaken for generic.
 *
 * Why the shared stack: when /graph mounts both panels at once, two same-phase
 * `document` Escape listeners meant one Esc closed BOTH — `stopPropagation` does
 * NOT stop sibling listeners on the same target/phase (see escLayering.test), and
 * the old per-panel guard (`defaultPrevented` + `stopImmediatePropagation`) closed
 * exactly one panel but WHICH one was registration-order-dependent (the FIRST-
 * mounted / outermost panel won). The stack makes it deterministic innermost-first:
 *
 *   - Each consumer still registers its OWN document/capture listener on mount.
 *     That deliberately preserves the registration timing the ego-graph fix
 *     (PSY-1351) relies on: in the Radix <Dialog>, Radix's DismissableLayer
 *     registers its document-capture Escape at dialog-open, BEFORE the panel
 *     mounts on edge-click, so Radix wins Escape and ArtistGraphDialog's
 *     onEscapeKeyDown intercepts. A single lazily-registered shared listener
 *     could flip that order; per-consumer registration cannot.
 *   - Only the stack-top listener acts; the others no-op. The winner
 *     preventDefaults + stopImmediatePropagation so the bubble-phase
 *     fullscreen-overlay listener (which skips defaultPrevented) defers.
 *
 * SCOPE LIMITATION: the stack is a single global LIFO with no notion of stacking
 * context — it coordinates panels that share ONE logical layer. Two INDEPENDENT
 * graph surfaces alive on the same page (e.g. the inline BillComposition graph
 * AND the Similar-Artists Radix dialog on an artist page) both push onto this one
 * stack, so a background panel can out-rank a modal's Escape. That
 * non-modal-vs-modal coordination is a pre-existing gap (the old per-panel
 * listeners collided the same way, worse) tracked in PSY-1368; it is out of
 * this hook's guarantee, which is innermost-first WITHIN one surface.
 *
 * `ignoreFromInput` reproduces ArtistContextPanel's guard.
 */

import { useEffect, useRef } from 'react'

/** Active dismissers, innermost (most-recently registered) last. */
const escapeStack: symbol[] = []

export interface UseGraphPanelEscapeOptions {
  /** When false the listener is not registered (e.g. an empty panel). Default true. */
  enabled?: boolean
  /**
   * Ignore Escapes whose target is a text control (input / textarea / select /
   * contenteditable) OR sits inside an open `[role="dialog"]`, so that control
   * dismisses itself first (e.g. a command palette). Default false. (Named for
   * the common case; the `[role="dialog"]` clause is the reason ConnectionPanel
   * opts OUT — see its call site.)
   */
  ignoreFromInput?: boolean
}

function isFromInput(target: EventTarget | null): boolean {
  return (
    target instanceof Element &&
    !!target.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')
  )
}

export function useGraphPanelEscape(
  onEscape: () => void,
  { enabled = true, ignoreFromInput = false }: UseGraphPanelEscapeOptions = {},
): void {
  // Hold onEscape in a ref so a changing callback identity never re-runs the
  // registration effect (its deps are [enabled, ignoreFromInput] only). A
  // re-run pops and re-pushes the token, which re-promotes this panel to
  // innermost — fine when it happens because the panel just became `enabled`
  // (it has just become visible/active, so owning Escape is correct), but NOT
  // something a mere onEscape identity change should trigger.
  const onEscapeRef = useRef(onEscape)
  useEffect(() => {
    onEscapeRef.current = onEscape
  })

  useEffect(() => {
    if (!enabled) return

    const token = Symbol('graph-panel-escape')
    escapeStack.push(token)

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      // Only the innermost (top of stack) layer handles the key; everyone else
      // defers so one Escape closes exactly one layer.
      if (escapeStack[escapeStack.length - 1] !== token) return
      if (ignoreFromInput && isFromInput(e.target)) return
      e.preventDefault()
      // stopImmediatePropagation, not stopPropagation: sibling listeners on the
      // same document/capture phase are NOT stopped by stopPropagation.
      e.stopImmediatePropagation()
      onEscapeRef.current()
    }

    document.addEventListener('keydown', onKeyDown, { capture: true })
    return () => {
      const i = escapeStack.indexOf(token)
      if (i !== -1) escapeStack.splice(i, 1)
      document.removeEventListener('keydown', onKeyDown, { capture: true })
    }
  }, [enabled, ignoreFromInput])
}
