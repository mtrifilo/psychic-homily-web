'use client'

import { Check } from 'lucide-react'

interface EntitySaveSuccessBannerProps {
  /**
   * When true, render the green-bordered "Changes saved" confirmation. Auto-
   * dismiss timing is owned by the consumer (typically `useEntitySaveSuccessBanner`).
   */
  visible: boolean
}

/**
 * Page-level confirmation banner shown after an admin / trusted-contributor
 * direct save via {@link EntityEditDrawer}. The drawer closes on direct save,
 * so the in-drawer success state isn't durable enough for the user to notice
 * when the edited field has no immediately-visible representation (e.g.
 * SoundCloud URL with no music embed configured). This banner persists on the
 * detail page itself.
 *
 * Mirrors the styling of the in-drawer "Edit submitted for review" sibling
 * banner in {@link EntityEditDrawer} so the visual vocabulary (green = success,
 * amber = pending) stays consistent across surfaces.
 */
export function EntitySaveSuccessBanner({ visible }: EntitySaveSuccessBannerProps) {
  if (!visible) return null

  return (
    <div
      role="status"
      aria-live="polite"
      className="mb-4 rounded-md border border-green-800 bg-green-950/50 p-4"
    >
      <div className="flex items-center gap-2 text-green-400">
        <Check className="h-4 w-4" aria-hidden="true" />
        <span className="font-medium">Changes saved</span>
      </div>
    </div>
  )
}
