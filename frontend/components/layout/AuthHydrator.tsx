import { HydrationBoundary } from '@tanstack/react-query'
import { prefetchAuthProfile } from '@/lib/auth-hydration'

interface AuthHydratorProps {
  children: React.ReactNode
}

/**
 * Server component that pre-seeds the TanStack Query cache for
 * `/auth/profile` so `useProfile()` resolves from cache on first paint.
 * Without the seed, hydrated detail pages paint instantly but
 * auth-gated action buttons (AttendanceButton, SaveButton, etc.) are
 * interactive while `isAuthenticated` is still `false` — a click before
 * the client profile fetch settles routes the user to
 * `/auth?returnTo=…` instead of firing the intended POST (the PSY-797
 * race).
 *
 * The cookie read happens inside `prefetchAuthProfile`, NOT in the root
 * layout — wrapping this component in `<Suspense>` (see
 * `app/layout.tsx`) lets PPR keep the static shell prerendered and
 * stream only this subtree dynamically. Reading cookies in the root
 * layout directly would opt every route into dynamic rendering and
 * defeat ISR on every page that sets `next: { revalidate: 3600 }`
 * (PSY-834's regression — see PSY-841 for the fix).
 *
 * Must be placed INSIDE `<Providers>` so `<HydrationBoundary>` has
 * access to the QueryClientProvider context.
 */
export async function AuthHydrator({ children }: AuthHydratorProps) {
  const dehydratedState = await prefetchAuthProfile()

  return (
    <HydrationBoundary state={dehydratedState}>{children}</HydrationBoundary>
  )
}
