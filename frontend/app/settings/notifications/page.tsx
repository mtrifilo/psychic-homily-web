/**
 * `/settings/notifications` → the Alerts card in profile settings.
 *
 * This path has been a redirect since PSY-595, which pointed it at the
 * filter manager to keep old bookmarks landing on the surface that then
 * owned notification preferences.
 *
 * PSY-1905 moved that ownership: the account alert matrix lives in profile
 * settings, and the filter manager is now the narrower "Custom alerts"
 * criteria path. PSY-1896 then made this path load-bearing again by linking
 * it as the "manage" CTA of every artist show-alert email, and what that
 * reader wants is the matrix and their per-follow scope — not the
 * criteria-filter builder they never opened.
 *
 * So the target follows the ownership rather than the history. Custom alerts
 * stays one click away: the matrix's own custom-alerts row links to it, as do
 * the side nav, the command palette and the notification inbox.
 */

import { redirect } from 'next/navigation'
import { ALERTS_HREF } from '@/components/shared/followAlertChoices'

export default function LegacyNotificationSettingsRedirect() {
  redirect(ALERTS_HREF)
}
