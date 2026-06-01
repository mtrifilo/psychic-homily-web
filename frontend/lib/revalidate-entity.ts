import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'

/**
 * Revalidate one ISR path, never throwing.
 *
 * A revalidation failure must not turn an already-persisted backend save
 * into an error response — errors are reported to Sentry instead. Shared by
 * the dedicated admin routes (PSY-936) and the proxy revalidation rules
 * engine (PSY-939, lib/proxy-revalidation.ts).
 *
 * Accepts either a concrete URL path (`/artists/bright-eyes`) or a dynamic
 * route pattern (`/shows/[slug]`). Patterns invalidate every cached page
 * under that route on its next visit — used for cascade invalidation when
 * the affected pages can't be enumerated from the mutation (PSY-941).
 *
 * `source` identifies the caller in Sentry (an entity type or rule name).
 */
export function safeRevalidatePath(path: string, source: string): void {
  try {
    if (isRoutePattern(path)) {
      revalidatePath(path, 'page')
    } else {
      revalidatePath(path)
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'isr-revalidation', source },
      extra: { path },
    })
  }
}

/**
 * Dynamic route patterns (containing a `[segment]`) must pass type 'page' to
 * revalidatePath; concrete URLs must not pass a type. Real slugs never
 * contain brackets (backend slugs are kebab-case alphanumerics), so the
 * bracket is an unambiguous discriminator.
 */
function isRoutePattern(path: string): boolean {
  return path.includes('[')
}

/**
 * Revalidate the ISR-cached detail page for an artist after a mutation.
 *
 * Entity detail pages are ISR-cached (revalidate: 3600) and re-seed the
 * client query cache via prefetchEntity on load. Without revalidation, a
 * reload within the window re-serves the pre-mutation page, making the
 * save appear lost (PSY-936).
 *
 * Never throws: a revalidation failure must not turn an already-persisted
 * backend save into an error response. Missing slugs and revalidation
 * errors are reported to Sentry instead.
 *
 * Server-only (route handlers / server actions). Used by the dedicated
 * admin music routes; proxy-routed mutations are covered by
 * lib/proxy-revalidation.ts (PSY-939).
 */
export function revalidateArtistDetail(slug: string | undefined | null): void {
  if (!slug) {
    Sentry.captureMessage('revalidateArtistDetail: missing slug, skipped', {
      level: 'warning',
      tags: { service: 'isr-revalidation', entity: 'artist' },
    })
    return
  }

  safeRevalidatePath(`/artists/${slug}`, 'artist')
}
