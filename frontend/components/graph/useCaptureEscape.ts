'use client'

/**
 * useCaptureEscape (PSY-1360) — a single, coordinated capture-phase Escape
 * dismiss for stacked floating graph panels (ArtistContextPanel + ConnectionPanel).
 *
 * Why a shared hook instead of a per-panel effect: when /graph mounts both
 * panels at once, two same-phase `document` Escape listeners meant one Esc
 * closed BOTH — `stopPropagation` does NOT stop sibling listeners on the same
 * target/phase (see escLayering.test), so the two panels papered over it with
 * a `defaultPrevented` guard + `stopImmediatePropagation`. That made one Esc
 * close exactly one panel, but WHICH one was registration-order-dependent
 * (the FIRST-mounted / outermost panel won) — not innermost-first.
 *
 * This hook coordinates through a module-level LIFO stack so one Escape
 * dismisses only the topmost (most-recently-mounted = innermost) layer:
 *
 *   - Each consumer still registers its OWN document/capture listener on mount.
 *     That deliberately preserves the registration timing the ego-graph fix
 *     (PSY-1351) relies on: in the Radix <Dialog>, Radix's DismissableLayer
 *     registers its document-capture Escape at dialog-open, BEFORE the panel
 *     mounts on edge-click, so Radix wins Escape and ArtistGraphDialog's
 *     onEscapeKeyDown intercepts. A single lazily-registered shared listener
 *     could flip that order; per-consumer registration cannot.
 *   - Only the stack-top listener acts; the others no-op (they let the
 *     innermost layer handle it). The winner preventDefaults +
 *     stopImmediatePropagation so the bubble-phase fullscreen-overlay listener
 *     (which skips defaultPrevented events) defers — innermost closes first.
 *
 * `ignoreFromInput` reproduces ArtistContextPanel's guard: an Escape typed into
 * an input / textarea / select / contenteditable / [role="dialog"] is left for
 * that control (e.g. a command palette dismissing itself), not consumed here.
 * ConnectionPanel opts out (its ego-dialog case is handled at the Dialog
 * boundary, and it must stay dismissable while the canvas has focus).
 */

import { useEffect, useRef } from 'react'

/** Active dismissers, innermost (most-recently-mounted) last. */
const escapeStack: symbol[] = []

export interface UseCaptureEscapeOptions {
  /** When false the listener is not registered (e.g. an empty panel). Default true. */
  enabled?: boolean
  /**
   * Ignore Escapes targeted at a text control or an open [role="dialog"] so the
   * control dismisses itself first. Default false.
   */
  ignoreFromInput?: boolean
}

function isFromInput(target: EventTarget | null): boolean {
  return (
    target instanceof Element &&
    !!target.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')
  )
}

export function useCaptureEscape(
  onEscape: () => void,
  { enabled = true, ignoreFromInput = false }: UseCaptureEscapeOptions = {},
): void {
  // Hold onEscape in a ref so a changing callback identity never re-runs the
  // effect — re-running would pop and re-push this token, wrongly promoting the
  // panel to innermost. The stack order must track mount order only.
  const onEscapeRef = useRef(onEscape)
  useEffect(() => {
    onEscapeRef.current = onEscape
  })

  useEffect(() => {
    if (!enabled) return

    const token = Symbol('capture-escape')
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
