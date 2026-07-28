import { HydrationBoundary } from '@tanstack/react-query'
import { prefetchAuthProfile } from '@/lib/auth-hydration'
import { SessionProviders } from './providers'

interface AuthHydratorProps {
  children: React.ReactNode
}

/**
 * Server component that pre-seeds the TanStack Query cache for
 * `/auth/profile` so `useProfile()` resolves from cache on first paint.
 * Without the seed, hydrated detail pages paint instantly but
 * auth-gated action buttons (SaveButton, etc.) are
 * interactive while `isAuthenticated` is still `false` — a click before
 * the client profile fetch settles routes the user to
 * `/auth?returnTo=…` instead of firing the intended POST.
 *
 * The cookie read happens inside `prefetchAuthProfile`, NOT in the root
 * layout — wrapping this component in `<Suspense>` (see
 * `app/layout.tsx`) lets PPR keep the static shell prerendered and
 * stream only this subtree dynamically. Reading cookies in the root
 * layout directly would opt every route into dynamic rendering and
 * defeat ISR on every page that sets `next: { revalidate: 3600 }`.
 *
 * Placement, and why the order is load-bearing: this must render INSIDE
 * `<Providers>` (so `<HydrationBoundary>` can reach the QueryClientProvider
 * context) and OUTSIDE `<SessionProviders>` — which it therefore mounts
 * itself, rather than leaving to the caller. `AuthProvider` reads the profile
 * through `useProfile()`, so anything auth-aware that renders above this
 * boundary sees an empty cache during the single server render and falls back
 * to its loading branch: the SSR HTML ships an empty spinner shell instead of
 * the signed-in tree (and, symmetrically, instead of the login link for
 * anonymous viewers). Keeping the provider inside the boundary is what makes
 * the server render the real tree on first paint.
 */
export async function AuthHydrator({ children }: AuthHydratorProps) {
  const dehydratedState = await prefetchAuthProfile()

  return (
    <HydrationBoundary state={dehydratedState}>
      <SessionProviders>{children}</SessionProviders>
    </HydrationBoundary>
  )
}
