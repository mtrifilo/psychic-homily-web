'use client'

/**
 * GraphSectionErrorBoundary (PSY-1359) — the shared error boundary for the
 * below-the-fold, lazily-mounted graph sections (HomeSceneGraph, InlineGraph).
 *
 * Why it exists: in the App Router, `next/dynamic(ssr:false)` does NOT re-invoke
 * `loading` with an `error` — a failed chunk fetch (e.g. a deploy rotated the
 * hashed chunk while the page was open) THROWS from React.lazy to the nearest
 * error boundary. Without a LOCAL one, that throw bubbles to app/error.tsx and
 * replaces the whole page. Each graph section is optional, so a graph failure
 * must be contained to the section, reported, and either self-hidden or shown
 * as a recoverable card — never allowed to take the page down.
 *
 * Parameterized by:
 *   - `sentryTag`: the `section` tag the failure is reported under, so the two
 *     surfaces are distinguishable in Sentry.
 *   - `fallback`: what to render on error. Omit it to SELF-HIDE (render nothing
 *     — the homepage's posture: the section just disappears). Provide it to show
 *     a visible, recoverable state (/explore's posture); it receives a `reset`
 *     that clears the error and re-attempts the children — for a transient chunk
 *     hiccup the retry can succeed; a deploy-skew rotation needs a full reload,
 *     which the fallback copy can point to.
 *
 * Class component because React error boundaries have no hook equivalent.
 */

import { Component, type ReactNode } from 'react'
import * as Sentry from '@sentry/nextjs'

interface GraphSectionErrorBoundaryProps {
  children: ReactNode
  /** Sentry `section` tag the failure is attributed to. */
  sentryTag: string
  /**
   * Rendered on error. Receives a `reset` that clears the error and re-renders
   * the children. Omit to self-hide (render nothing).
   */
  fallback?: (reset: () => void) => ReactNode
}

interface GraphSectionErrorBoundaryState {
  failed: boolean
}

export class GraphSectionErrorBoundary extends Component<
  GraphSectionErrorBoundaryProps,
  GraphSectionErrorBoundaryState
> {
  state: GraphSectionErrorBoundaryState = { failed: false }

  static getDerivedStateFromError(): GraphSectionErrorBoundaryState {
    return { failed: true }
  }

  componentDidCatch(error: unknown) {
    // Self-hiding must not mean silent: a systematic chunk failure (deploy skew,
    // CDN flake) would otherwise kill the section for everyone with nothing
    // reported (app/global-error.tsx never sees it — this boundary caught it).
    Sentry.captureException(error, {
      tags: { section: this.props.sentryTag },
    })
  }

  reset = () => {
    this.setState({ failed: false })
  }

  render() {
    if (this.state.failed) {
      return this.props.fallback ? this.props.fallback(this.reset) : null
    }
    return this.props.children
  }
}
