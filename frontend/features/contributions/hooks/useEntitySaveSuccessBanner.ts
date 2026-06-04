'use client'

import { useCallback } from 'react'
import { useAutoDismissFlag } from '@/lib/hooks/common/useAutoDismissBanner'
import type { EntityEditSuccess } from '../types'

/**
 * Auto-dismiss delay for the page-level "Changes saved" banner that follows a
 * direct admin/trusted save in {@link EntityEditDrawer}. Long enough to read,
 * short enough to stay out of the way (PSY-562 ticket default).
 */
const AUTO_DISMISS_MS = 5000

interface UseEntitySaveSuccessBannerResult {
  /**
   * True while the success banner should be visible. Drives the conditional
   * render of {@link EntitySaveSuccessBanner}.
   */
  isVisible: boolean
  /**
   * Wire this into {@link EntityEditDrawer}'s `onSuccess` prop. The drawer
   * passes `{ applied }`; we only flash the banner on direct saves (the
   * pending-review path keeps the in-drawer amber banner instead).
   */
  handleSaveSuccess: (result: EntityEditSuccess) => void
}

/**
 * Manages the page-level success-banner state for entity-detail pages that
 * host an {@link EntityEditDrawer}.
 *
 * Why a hook (not a `<Banner>` ref-component): the banner needs to outlive the
 * drawer (which closes on direct save) and live on the detail page itself, so
 * the timer + state must be owned by the page component. Putting it behind a
 * hook keeps the wiring at each of the 6 entity detail pages to one line.
 *
 * PSY-958: the show-then-auto-dismiss timer is the shared
 * {@link useAutoDismissFlag} primitive now; this hook keeps its name + shape
 * (the 6 detail-page consumers are untouched) and adds only the
 * `result.applied` gate on top. Behavior note: a second direct save while the
 * banner is still up now RE-ARMS the 5s window (the primitive re-arms on every
 * `trigger()`); the pre-PSY-958 effect keyed on `[isVisible]` did not, so it
 * let the original window run out. The re-armed behavior is the more correct
 * one (each save gets a full window).
 */
export function useEntitySaveSuccessBanner(): UseEntitySaveSuccessBannerResult {
  const [isVisible, trigger] = useAutoDismissFlag(AUTO_DISMISS_MS)

  const handleSaveSuccess = useCallback(
    (result: EntityEditSuccess) => {
      // Only flash on direct saves. Pending submissions keep the in-drawer
      // amber "submitted for review" banner — the drawer stays open.
      if (result.applied) {
        trigger()
      }
    },
    [trigger]
  )

  return { isVisible, handleSaveSuccess }
}
