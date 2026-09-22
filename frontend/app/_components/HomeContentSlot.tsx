// Deep imports, not the `@/features/home` barrel: this file pulls in BOTH
// variants, and a `'use client'` barrel is not tree-shaken per export here.
// See features/sharedChunkBarrelGuard.test.ts.
import { AnonymousHome } from '@/features/home/components/AnonymousHome'
import { SignedInHome } from '@/features/home/components/SignedInHome'
import { isAuthenticatedViewer } from '@/lib/auth-hydration'

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
 * The read shares `AppShell`'s nav-mode `React.cache()`, so the variant costs
 * no extra backend fetch.
 *
 * The anonymous branch is the fallback for an unresolved read as well as for a
 * genuinely logged-out one. See {@link isAuthenticatedViewer}.
 */
export async function HomeContentSlot() {
  const isSignedIn = await isAuthenticatedViewer()
  return isSignedIn ? <SignedInHome /> : <AnonymousHome />
}
