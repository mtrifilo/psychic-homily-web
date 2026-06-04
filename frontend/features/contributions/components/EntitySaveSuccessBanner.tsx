'use client'

import { StatusBanner } from '@/components/shared'

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
 * PSY-575: now a thin wrapper around the shared `StatusBanner` primitive so
 * the green-vs-amber visual vocabulary stays consistent across all surfaces
 * (in-drawer admin save, page-level save, ReportEntityDialog success,
 * pending-review banners on comments + field notes).
 */
export function EntitySaveSuccessBanner({
  visible,
  message = DEFAULT_MESSAGE,
}: EntitySaveSuccessBannerProps) {
  if (!visible) return null

  return (
    <StatusBanner variant="success" className="mb-4">
      <span className="font-medium text-success-foreground">{message}</span>
    </StatusBanner>
  )
}
