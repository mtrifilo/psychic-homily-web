import { AnonymousHome, SignedInHome } from '@/features/home'
import { isAuthenticatedViewer } from '@/lib/auth-hydration'

/**
 * Picks the homepage's viewer variant on the SERVER (PSY-2103), so a signed-in
 * viewer's first paint is already their page and nothing re-branches after
 * hydration.
 *
 * `app/page.tsx` renders this inside its own `<Suspense>`: the cookie read here
 * is the page's only dynamic input, so the route keeps its partially-prerendered
 * shell and streams just this subtree. Same shape as `AppShell`'s nav-mode read
 * in the root layout, and it shares that read's `React.cache()` — the variant
 * costs no extra backend fetch.
 *
 * The anonymous branch is the fallback for an unresolved read as well as for a
 * genuinely logged-out one. See {@link isAuthenticatedViewer}.
 */
export async function HomeContentSlot() {
  const isSignedIn = await isAuthenticatedViewer()
  return isSignedIn ? <SignedInHome /> : <AnonymousHome />
}
