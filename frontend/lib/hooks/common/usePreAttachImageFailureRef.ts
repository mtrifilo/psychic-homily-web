'use client'

import { useCallback, useInsertionEffect, useRef } from 'react'

/**
 * A callback ref for an `<img>` that reports a load failure `onError`
 * structurally could not see (PSY-1685, generalised in PSY-1719).
 *
 * `onError` only catches a failure that happens AFTER React attached the
 * handler. Whenever the element is already in the document by the time React
 * reaches it — every image present in server-rendered HTML, because the
 * browser starts fetching as soon as it parses the tag — a dead hotlink can
 * 404 first, and that error event is fired at nobody. The component is then
 * stuck believing an image is on its way that is never coming, and shows a
 * blank slot instead of its fallback.
 *
 * What outlives the event is the element's own state, so this reads it at the
 * moment the node exists: `complete` with a zero `naturalWidth` is exactly
 * "finished, and decoded nothing". An image still in flight reports
 * `complete === false`, so the healthy case is untouched — and that half of
 * the predicate is load-bearing, not defensive padding. Dropping it would
 * blank every image the browser has not decoded yet, which on a cold page is
 * all of them.
 *
 * **This does not replace `onError`; wire up both.** The ref fires once per
 * attached node and only reports what already happened, so every failure
 * after that still arrives through the handler — which on a client-navigated
 * route is most of them.
 *
 * The empty-`src` guard is not a formality. Per the HTML spec `complete` is
 * TRUE when `src` is absent or the empty string, so without it an `<img>`
 * that has not been given a URL yet reports as a failure and its owner
 * permanently shows a fallback for an image that was never requested. The
 * attribute is read rather than the `src` property because the property
 * resolves `src=""` against the document URL and comes back truthy.
 *
 * `onFailure` may be an inline arrow: it is held in a ref, so the returned
 * callback is identity-stable for the life of the component and React
 * attaches it exactly once per node. That matters more than it looks. A ref
 * whose identity changed on every render would be detached and re-attached on
 * every render, and re-running this read has two failure modes: against a
 * REUSED element `complete`/`naturalWidth` still describe the PREVIOUS image
 * (the spec queues the image update as a microtask), so a good replacement
 * gets judged on the old one's failure; and if `onFailure` sets state to a
 * fresh value each call while the element stays mounted, React cannot bail
 * out and the subtree loops until it throws "Maximum update depth exceeded".
 *
 * `useInsertionEffect`, specifically, is what makes the ref safe to hold: it
 * runs BEFORE refs are attached, on remount as well as mount, so the callback
 * this reads is never a render stale. A passive `useEffect` — the latest-ref
 * idiom `useDismissTimer` uses — would be one render behind every time the
 * node remounts, because passive effects run after ref attachment. This is
 * the same mechanism React's own `useEffectEvent` is built on.
 *
 * Known false positive, accepted: Firefox reports `naturalWidth` 0 for an SVG
 * with no intrinsic width, so such an SVG collapses to the fallback there even
 * though it rendered. For the flyer callers this is close to theoretical —
 * flyers are scraped photographs and scans. For `CollectionCoverImage` it is
 * genuinely reachable: `cover_image_url` is a free-text field and the backend
 * caps its length without checking format (`shared/url_validation.go`), so a
 * user can set an SVG cover. Accepted because the fallback is a designed state
 * rather than a broken box; the honest fix, if it ever matters, is a format
 * guard on the field, not a guess at the URL here.
 *
 * A callback ref rather than an effect: this is reading the DOM node the
 * moment it exists, which is what a ref is for.
 *
 * @param onFailure Called when the node has already finished and decoded
 *   nothing. Callers own what that means — collapse a column, swap in a
 *   typographic tile, record the failed URL. Need not be memoised.
 * @returns An identity-stable ref callback to put on the `<img>`.
 */
export function usePreAttachImageFailureRef(
  onFailure: () => void
): (node: HTMLImageElement | null) => void {
  const latestOnFailure = useRef(onFailure)
  useInsertionEffect(() => {
    latestOnFailure.current = onFailure
  })

  return useCallback((node: HTMLImageElement | null) => {
    if (!node?.getAttribute('src')) return
    if (node.complete && node.naturalWidth === 0) latestOnFailure.current()
  }, [])
}
