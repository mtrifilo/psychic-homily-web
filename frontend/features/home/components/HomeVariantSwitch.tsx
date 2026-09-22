'use client'

import { useAuthContext } from '@/lib/context/AuthContext'
import { AnonymousHome } from './AnonymousHome'
import { SignedInHome } from './SignedInHome'

/**
 * The fallback for a homepage whose server-side viewer read could not answer
 * (backend blip, interstitial). Paints the anonymous page, which is right for
 * most such viewers, and swaps to the signed-in page only once the viewer's
 * own profile query settles authenticated. Used ONLY on that path: a viewer the
 * server did name never sees this component, so their page never re-branches.
 *
 * No server-read layout to pass: the read that would have carried it is the
 * one that failed. The client reader takes the layout off the profile query
 * instead, which has settled by the time this branch renders the signed-in
 * page (PSY-2104).
 */
export function HomeVariantSwitch() {
  const { authStatus } = useAuthContext()
  return authStatus === 'authenticated' ? (
    <SignedInHome layout={null} />
  ) : (
    <AnonymousHome />
  )
}
