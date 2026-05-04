'use client'

import { useEffect, useState, useCallback } from 'react'

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
  handleSaveSuccess: (result: { applied: boolean }) => void
}

/**
 * Manages the page-level success-banner state for entity-detail pages that
 * host an {@link EntityEditDrawer}.
 *
 * Why a hook (not a `<Banner>` ref-component): the banner needs to outlive the
 * drawer (which closes on direct save) and live on the detail page itself, so
 * the timer + state must be owned by the page component. Putting it behind a
 * hook keeps the wiring at each of the 5 detail pages to one line.
 */
export function useEntitySaveSuccessBanner(): UseEntitySaveSuccessBannerResult {
  const [isVisible, setIsVisible] = useState(false)

  const handleSaveSuccess = useCallback((result: { applied: boolean }) => {
    // Only flash on direct saves. Pending submissions keep the in-drawer
    // amber "submitted for review" banner — the drawer stays open.
    if (result.applied) {
      setIsVisible(true)
    }
  }, [])

  useEffect(() => {
    if (!isVisible) return

    const timer = setTimeout(() => {
      setIsVisible(false)
    }, AUTO_DISMISS_MS)

    return () => clearTimeout(timer)
  }, [isVisible])

  return { isVisible, handleSaveSuccess }
}
