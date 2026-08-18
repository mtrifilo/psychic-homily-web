'use client'

import { useCallback } from 'react'

/**
 * A ref for an `<img>` that reports a load failure `onError` structurally
 * could not see (PSY-1685, generalised in PSY-1719).
 *
 * `onError` only catches a failure that happens AFTER React attached the
 * handler. Whenever the element is already in the document by the time React
 * reaches it — which is every server-rendered image, because the browser
 * starts fetching as soon as it parses the tag — a dead hotlink can 404 first,
 * and that error event is fired at nobody. The component is then stuck
 * believing an image is on its way that is never coming, and shows a blank
 * slot instead of its fallback.
 *
 * What outlives the event is the element's own state, so this reads it once,
 * at the moment the node exists: `complete` with a zero `naturalWidth` is
 * exactly "finished, and decoded nothing". An image still in flight reports
 * `complete === false`, so the healthy case is untouched — and that half of
 * the predicate is load-bearing, not defensive padding. Dropping it would
 * blank every image the browser has not decoded yet, which in jsdom and on a
 * cold page is all of them.
 *
 * Pair it with `onError`; it does not replace it. This fires once per mounted
 * node, and a failure that happens later still needs the handler.
 *
 * Known false positive, accepted: Firefox reports `naturalWidth` 0 for an SVG
 * with no intrinsic width, so an SVG that rendered fine collapses to the
 * fallback there. The alternatives — sniffing the extension, reading the
 * content type — are guesses about a URL, and every current caller shows
 * photographs and scans. Revisit if a caller starts serving vector art.
 *
 * A callback ref rather than an effect: this is reading the DOM node the
 * moment it exists, which is what a ref is for.
 *
 * @param onFailure Called when the node has already finished and decoded
 *   nothing. Callers own what that means — collapse a column, swap in a
 *   typographic tile, record the failed URL. Keep it stable (a `useState`
 *   setter, or your own `useCallback`) or the ref reattaches each render;
 *   reattaching is harmless but pointless.
 */
export function useImageFailedBeforeAttach(
  onFailure: () => void
): (node: HTMLImageElement | null) => void {
  return useCallback(
    (node: HTMLImageElement | null) => {
      if (node?.complete && node.naturalWidth === 0) onFailure()
    },
    [onFailure]
  )
}
