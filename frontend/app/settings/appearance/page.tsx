'use client'

/**
 * /settings/appearance — user-facing appearance preferences.
 *
 * Currently hosts the navigation-style toggle (PSY-1117): top bar (default) vs.
 * left sidebar — the missing piece that makes the PSY-1116 nav-mode shell
 * user-switchable.
 *
 * This page is auth-gated, so only authenticated users reach it. How a change
 * propagates for them:
 *   1. The switch flips instantly from a local optimistic override — immediate
 *      feedback, no wait on the network.
 *   2. The choice is PATCHed to the account (`nav_mode`). The ACCOUNT is the
 *      source of truth that drives the shell: AppShell resolves account > cookie
 *      at SSR (see getAuthenticatedNavMode), so for an authenticated viewer the
 *      account — not the cookie — is what renders the nav on first paint and
 *      after a refresh. This is what makes the preference cross-device + flash-free.
 *   3. On success we also write the `nav_mode` cookie — NOT to drive the
 *      authenticated shell (the account wins there) but as continuity for the
 *      logged-out/anonymous view on this browser, and as the fallback AppShell
 *      uses if the account read is unavailable (backend outage).
 *   4. router.refresh() re-renders the server shell, which re-reads the
 *      now-saved account and flips the nav chrome.
 */

import { useState } from 'react'
import { redirect, useRouter } from 'next/navigation'
import { Loader2, Check } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useUpdateProfile } from '@/features/auth'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { InlineErrorBanner } from '@/components/shared'
import { parseNavMode, setNavModeCookie, type NavMode } from '@/lib/nav-mode'

export default function AppearanceSettingsPage() {
  const { isAuthenticated, isLoading, user } = useAuthContext()
  const router = useRouter()
  const updateProfile = useUpdateProfile()

  // The saved account preference is the source of truth for the control.
  const accountMode = parseNavMode(user?.nav_mode)

  // `optimistic` is a transient override shown between a toggle and the account
  // catching up to it, so the switch gives instant feedback. The displayed mode
  // derives from it — there is no long-lived `mode` state to drift out of sync,
  // and an unrelated profile refetch (same nav_mode, new object ref) cannot
  // clobber an in-flight choice the way a reference-keyed guard would.
  const [optimistic, setOptimistic] = useState<NavMode | null>(null)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mode = optimistic ?? accountMode

  // Drop the override once the saved account has caught up to it (post-PATCH
  // refetch). Adjust-state-during-render — converges in one pass (clearing the
  // override makes the condition false). A failed save clears it in the catch
  // instead, reverting to the live account value.
  if (optimistic !== null && accountMode === optimistic) {
    setOptimistic(null)
  }

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!isAuthenticated) {
    redirect('/auth')
  }

  const handleChange = async (checked: boolean) => {
    const next: NavMode = checked ? 'side' : 'top'

    setError(null)
    setSaved(false)
    setOptimistic(next) // instant switch feedback

    try {
      await updateProfile.mutateAsync({ nav_mode: next })
      // Write the cookie only once the account is durably saved — its job is
      // logged-out/anonymous continuity + the backend-outage fallback, not the
      // authenticated flip (the account drives that). Writing it before the
      // save confirmed would persist a choice the account never accepted.
      setNavModeCookie(next)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
      // Re-render the server shell so the nav chrome flips to the saved account.
      router.refresh()
    } catch (err: unknown) {
      // Clearing the override reverts the switch to the live account value
      // (unchanged, since the save failed). No cookie was written, so there is
      // nothing to revert.
      setOptimistic(null)
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to update appearance settings'
      )
    }
  }

  return (
    <div className="container mx-auto max-w-3xl px-4 py-6">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">Appearance</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Customize how Psychic Homily looks for you.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Navigation style</CardTitle>
          <CardDescription>
            Choose where the primary navigation lives. Your choice is saved to
            your account and follows you across devices.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="nav-mode-switch">Side navigation</Label>
              <p className="text-sm text-muted-foreground">
                Use a left sidebar for navigation instead of the top bar.
              </p>
            </div>
            <Switch
              id="nav-mode-switch"
              checked={mode === 'side'}
              onCheckedChange={handleChange}
              disabled={updateProfile.isPending}
              aria-label="Use side navigation"
            />
          </div>

          {error && <InlineErrorBanner>{error}</InlineErrorBanner>}

          {saved && !error && (
            <p
              role="status"
              className="flex items-center gap-1 text-sm text-green-600 dark:text-green-400"
            >
              <Check className="h-4 w-4" aria-hidden="true" />
              Saved
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
