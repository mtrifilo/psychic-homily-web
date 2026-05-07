'use client'

import { Check } from 'lucide-react'

const DEFAULT_MESSAGE = 'Changes saved'

interface EntitySaveSuccessBannerProps {
  /**
   * When true, render the green-bordered success confirmation. Auto-dismiss
   * timing is owned by the consumer (typically `useEntitySaveSuccessBanner`
   * for entity-detail saves, or per-surface state for admin moderation flows).
   */
  visible: boolean
  /**
   * Optional override for the banner copy. Defaults to "Changes saved" — the
   * original entity-detail-page wording introduced by PSY-562.
   *
   * Generalized in PSY-622 so admin moderation surfaces (Approve / Reject in
   * {@link ModerationQueue}, future Resolve-report / Hide-field-note flows,
   * etc.) can reuse the same primitive instead of forking a near-identical
   * banner per call site. Keep messages short — this is positive feedback,
   * not a description; the visual treatment carries the "success" semantics.
   */
  message?: string
}

/**
 * Green success banner shown after a successful save / moderation action.
 *
 * Originally introduced (PSY-562) as a page-level confirmation for admin /
 * trusted-contributor direct saves via {@link EntityEditDrawer}: the drawer
 * closes on direct save, so the in-drawer success state isn't durable enough
 * for the user to notice when the edited field has no immediately-visible
 * representation (e.g. SoundCloud URL with no music embed configured), and
 * this banner persists on the detail page itself.
 *
 * Generalized in PSY-622 with an optional `message` prop so admin moderation
 * surfaces (`ModerationQueue` Approve / Reject, future Resolve-report,
 * Hide-field-note, Approve-attendance, …) can render the same primitive with
 * action-specific copy.
 *
 * Mirrors the styling of the in-drawer "Edit submitted for review" sibling
 * banner in {@link EntityEditDrawer} so the visual vocabulary (green = success,
 * amber = pending) stays consistent across surfaces.
 */
export function EntitySaveSuccessBanner({
  visible,
  message = DEFAULT_MESSAGE,
}: EntitySaveSuccessBannerProps) {
  if (!visible) return null

  return (
    <div
      role="status"
      aria-live="polite"
      className="mb-4 rounded-md border border-green-800 bg-green-950/50 p-4"
    >
      <div className="flex items-center gap-2 text-green-400">
        <Check className="h-4 w-4" aria-hidden="true" />
        <span className="font-medium">{message}</span>
      </div>
    </div>
  )
}
