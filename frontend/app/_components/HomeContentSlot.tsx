import { HydrationBoundary } from '@tanstack/react-query'
// Deep imports, not the `@/features/home` barrel: this file pulls in BOTH
// variants, and a `'use client'` barrel is not tree-shaken per export here.
// See features/sharedChunkBarrelGuard.test.ts.
import { AnonymousHome } from '@/features/home/components/AnonymousHome'
import { HomeVariantSwitch } from '@/features/home/components/HomeVariantSwitch'
import { SignedInHome } from '@/features/home/components/SignedInHome'
import {
  getAuthenticatedHomeLayout,
  prefetchHomeSavedShows,
  resolveHomeViewer,
} from '@/lib/auth-hydration'

/**
 * Picks the homepage's viewer variant on the SERVER (PSY-2103), so a signed-in
 * viewer's first paint is already their page and nothing re-branches after
 * hydration.
 *
 * `app/page.tsx` renders this inside its own `<Suspense>`. That boundary is a
 * STREAMING boundary, not what makes the route dynamic: `app/layout.tsx`
 * already wraps every page's children in a Suspense around `<AuthHydrator>`,
 * which awaits `prefetchAuthProfile()` and therefore `cookies()`, so the page
 * body has always rendered inside that postponed region. What the inner
 * boundary buys is that the variant swap suspends on its own instead of the
 * page's markup.
 *
 * The profile read shares `AppShell`'s `React.cache()`, so the variant costs
 * no extra backend fetch. A signed-in viewer's saved rows are prefetched here
 * too, so the module's first paint is its rows rather than a skeleton, and the
 * same cached read supplies their section layout, so a custom order is in the
 * first server HTML rather than reflowing after hydration.
 *
 * Three answers, three renders: a named viewer gets their page; an answered
 * "nobody" gets the anonymous page; a read the backend could not answer gets
 * the anonymous page with a client-side switch, so the viewer whose cookie is
 * valid still reaches their page once their own profile query settles. See
 * {@link resolveHomeViewer}.
 */
export async function HomeContentSlot() {
  const viewer = await resolveHomeViewer()
  if (viewer === 'authenticated') {
    return (
      <HydrationBoundary state={await prefetchHomeSavedShows()}>
        <SignedInHome layout={await getAuthenticatedHomeLayout()} />
      </HydrationBoundary>
    )
  }
  if (viewer === 'indeterminate') return <HomeVariantSwitch />
  return <AnonymousHome />
}
