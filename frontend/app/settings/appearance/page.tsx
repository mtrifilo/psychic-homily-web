'use client'

/**
 * /settings/appearance — user-facing appearance preferences.
 *
 * Today this hosts the navigation-style toggle (PSY-1117): top bar (default)
 * vs. left sidebar. The toggle is the missing piece that makes the PSY-1116
 * nav-mode shell actually switchable by users.
 *
 * Persistence is two-layered (the PSY-1116 decision):
 *   • cookie  — written client-side so the server shell (AppShell) flips on the
 *     next render; `router.refresh()` makes that immediate.
 *   • account — PATCHed to `nav_mode` so the preference follows the user across
 *     devices. AppShell reads the account value first (see getAuthenticatedNavMode),
 *     so a fresh browser with no cookie still renders the saved nav on first paint.
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

// Sentinel so the re-seed guard below fires on the FIRST render too (mirrors the
// adjust-state-during-render idiom in app/profile/page.tsx). Tracks the last
// account value the switch was synced to.
const UNSET = Symbol('unset')

export default function AppearanceSettingsPage() {
  const { isAuthenticated, isLoading, user } = useAuthContext()
  const router = useRouter()
  const updateProfile = useUpdateProfile()

  const [mode, setMode] = useState<NavMode>('top')
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Re-seed the control from the saved account preference whenever it changes
  // (async profile load, or a fresh user reference after our own save). React
  // 19.2 adjust-state-during-render idiom — same pattern as app/profile/page.tsx,
  // avoiding a mount/update effect.
  const accountMode = parseNavMode(user?.nav_mode)
  const [prevAccountMode, setPrevAccountMode] = useState<
    NavMode | typeof UNSET
  >(UNSET)
  if (user && accountMode !== prevAccountMode) {
    setPrevAccountMode(accountMode)
    setMode(accountMode)
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
    const previous = mode

    setError(null)
    setSaved(false)
    setMode(next) // optimistic
    setNavModeCookie(next) // instant local write-through for the server shell

    try {
      await updateProfile.mutateAsync({ nav_mode: next })
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
      // Re-render the server shell so the nav chrome flips immediately without a
      // full navigation. AppShell re-reads the (now-updated) account + cookie.
      router.refresh()
    } catch (err: unknown) {
      // Revert the optimistic UI and the cookie so the visible state matches the
      // still-unchanged account.
      setMode(previous)
      setNavModeCookie(previous)
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
            <p className="flex items-center gap-1 text-sm text-green-600 dark:text-green-400">
              <Check className="h-4 w-4" aria-hidden="true" />
              Saved
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
