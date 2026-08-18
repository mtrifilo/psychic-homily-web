'use client'

import { useCallback, useInsertionEffect, useRef } from 'react'

/**
 * A callback ref for an `<img>` that reports a load failure `onError`
 * structurally could not see.
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
 * **This does not replace `onError`; wire up both.** The ref only reports what
 * already happened, so every failure after attachment still arrives through
 * the handler — which on a client-navigated route is most of them.
 *
 * **`onFailure` must be idempotent.** It fires once per ATTACHMENT, which is
 * not the same as once per node: React detaches and re-attaches refs on the
 * same element under StrictMode's double-invoke (on by default in Next dev)
 * and on every Suspense or Activity reveal. Setting a flag or recording the
 * failed URL is fine; incrementing a counter or firing a beacon would
 * double-count.
 *
 * The empty-`src` guard is not a formality. Per the HTML spec `complete` is
 * TRUE when `src` is absent or the empty string, so without it an `<img>`
 * that has not been given a URL yet reports as a failure and its owner
 * permanently shows a fallback for an image that was never requested. The
 * attribute is read rather than the `src` property because the property
 * resolves `src=""` against the document URL and comes back truthy.
 *
 * `onFailure` may be an inline arrow: it is held in a ref, so the returned
 * callback is identity-stable for the life of the component. That matters
 * more than it looks. A ref whose identity changed on every render would be
 * detached and re-attached on every render, and re-running this read has two
 * failure modes: against a REUSED element `complete`/`naturalWidth` still
 * describe the PREVIOUS image (the spec queues the image update as a
 * microtask), so a good replacement gets judged on the old one's failure; and
 * if `onFailure` sets state to a fresh value each call while the element stays
 * mounted, React cannot bail out and the subtree loops until it throws
 * "Maximum update depth exceeded". Both were measured before this shape.
 *
 * `useInsertionEffect`, specifically, is what makes the ref safe to hold: it
 * runs in the mutation phase, BEFORE refs are attached in the layout phase, on
 * remount as well as mount, so the callback this reads is never a render
 * stale. A passive `useEffect` — the latest-ref idiom `useDismissTimer` uses —
 * would be one render behind every time the node remounts, because passive
 * effects run after ref attachment. The remount test pins this; it fails if
 * the write is moved to `useEffect` or `useLayoutEffect`.
 *
 * Known false positive, accepted: Firefox reports `naturalWidth` 0 for an SVG
 * with no intrinsic width, so such an SVG collapses to the fallback there even
 * though it rendered. For the flyer callers this is close to theoretical —
 * flyers are scraped photographs and scans. For `CollectionCoverImage` it is
 * genuinely reachable: `shared.ValidateURLField` checks that `cover_image_url`
 * is an http(s) URL within a length cap, but nothing checks the image FORMAT,
 * so a user can set an `.svg` cover. Accepted because the fallback is a
 * designed state rather than a broken box; the honest fix, if it ever matters,
 * is a format guard on the field, not a guess at the URL here.
 *
 * A callback ref rather than an effect: this is reading the DOM node the
 * moment it exists, which is what a ref is for.
 *
 * @param onFailure Called when the node has already finished and decoded
 *   nothing. Callers own what that means — collapse a column, swap in a
 *   typographic tile, record the failed URL. Need not be memoised, but must
 *   be idempotent (see above).
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
