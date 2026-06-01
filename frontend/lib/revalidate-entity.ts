import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'

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
 * Server-only (route handlers / server actions). PSY-937 extends this
 * pattern to the remaining entity types and the proxy-routed mutations.
 */
export function revalidateArtistDetail(slug: string | undefined | null): void {
  if (!slug) {
    Sentry.captureMessage('revalidateArtistDetail: missing slug, skipped', {
      level: 'warning',
      tags: { service: 'isr-revalidation', entity: 'artist' },
    })
    return
  }

  try {
    revalidatePath(`/artists/${slug}`)
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'isr-revalidation', entity: 'artist' },
      extra: { slug },
    })
  }
}
