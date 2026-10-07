'use client'

/**
 * CanvasSectionErrorBoundary: the shared error boundary for a lazily mounted
 * canvas section, a graph or a map.
 *
 * Why it exists: in the App Router, `next/dynamic(ssr:false)` does NOT re-invoke
 * `loading` with an `error`: a failed chunk fetch (e.g. a deploy rotated the
 * hashed chunk while the page was open) THROWS from React.lazy to the nearest
 * error boundary. Without a LOCAL one, that throw bubbles to app/error.tsx and
 * replaces the whole page. Each section is optional, so its failure must be
 * contained to the section, reported, and either self-hidden or shown as a
 * recoverable card, never allowed to take the page down.
 *
 * Parameterized by:
 *   - `sentryTag`: the `section` tag the failure is reported under, so the
 *     surfaces are distinguishable in Sentry.
 *   - `errorTags`: optional extra Sentry tags read from the caught error, for a
 *     consumer whose errors carry their own classification (a map tags how
 *     it failed). Tags are indexed and not scrubbed, so values come
 *     from a fixed vocabulary, never from error text or URLs. The `section`
 *     tag always wins a name clash, and a throwing `errorTags` costs only its
 *     own tags.
 *   - `fallback`: what to render on error. Omit it to SELF-HIDE (render nothing —
 *     the homepage's posture: the section just disappears). Provide a node to show
 *     a visible state (/explore's posture).
 *   - `onError`: optional notification, called once from componentDidCatch after
 *     the Sentry report, so a SELF-HIDING consumer can react outside this
 *     boundary: retract copy that only makes sense with the canvas present (a
 *     "click a name"-style instruction above it), or replace the whole surface
 *     with the consumer's own fallback, as the Atlas does with its scene list
 *     (it passes no `fallback`, so this boundary's latch stays its own). What
 *     this boundary itself renders is unchanged either way.
 *
 * NOTE on recovery: the boundary deliberately does NOT offer an in-place "reset".
 * next/dynamic wraps the import in a module-scoped React.lazy that permanently
 * caches a rejected import (it re-throws the cached error without re-invoking the
 * loader), so re-rendering the same lazy after a reset just re-throws. And the
 * dominant real failure is a deploy rotating the hashed chunk — the open page's
 * baked-in chunk URL then 404s no matter how often it re-imports; only fresh HTML
 * carries the new URL. So the only reliable recovery is a full page reload, which
 * a visible fallback can offer (see InlineGraph); the homepage just self-hides.
 *
 * Class component because React error boundaries have no hook equivalent.
 */

import { Component, type ReactNode } from 'react'
import * as Sentry from '@sentry/nextjs'

interface CanvasSectionErrorBoundaryProps {
  children: ReactNode
  /** Sentry `section` tag the failure is attributed to. */
  sentryTag: string
  /** Rendered on error. Omit to self-hide (render nothing). */
  fallback?: ReactNode
  /** Notified once when a failure is caught, after it is reported to Sentry. */
  onError?: (error: unknown) => void
  /** Extra Sentry tags for the caught error, beside `section`; fixed values only. */
  errorTags?: (error: unknown) => Record<string, string> | undefined
}

interface CanvasSectionErrorBoundaryState {
  failed: boolean
}

export class CanvasSectionErrorBoundary extends Component<
  CanvasSectionErrorBoundaryProps,
  CanvasSectionErrorBoundaryState
> {
  state: CanvasSectionErrorBoundaryState = { failed: false }

  static getDerivedStateFromError(): CanvasSectionErrorBoundaryState {
    return { failed: true }
  }

  componentDidCatch(error: unknown) {
    // Self-hiding must not mean silent: a systematic chunk failure (deploy skew,
    // CDN flake) would otherwise kill the section for everyone with nothing
    // reported (app/global-error.tsx never sees it — this boundary caught it).
    let extraTags: Record<string, string> | undefined
    try {
      extraTags = this.props.errorTags?.(error)
    } catch {
      // The report and onError still run, without the extra tags.
    }
    Sentry.captureException(error, {
      tags: { ...extraTags, section: this.props.sentryTag },
    })
    this.props.onError?.(error)
  }

  render() {
    if (this.state.failed) {
      return this.props.fallback ?? null
    }
    return this.props.children
  }
}
