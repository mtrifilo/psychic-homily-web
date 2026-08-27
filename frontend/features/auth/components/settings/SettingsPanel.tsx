'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useExportData, useGenerateCLIToken } from '@/features/auth'
// Relative import rather than the feature barrel: the barrel is mocked wholesale
// by this component's own suite, and the resend control is worth exercising for
// real.
import {
  VerificationResend,
  VerificationResendAlerts,
  VerificationResendButton,
  VerificationResendStatus,
} from '../verification-resend'
import { buildAuthHref } from '@/lib/auth-href'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  CheckCircle2,
  AlertCircle,
  Loader2,
  AlertTriangle,
  Download,
  Terminal,
  Copy,
  Check,
} from 'lucide-react'
import { ChangePassword } from './change-password'
import { DeleteAccountDialog } from './delete-account-dialog'
import { OAuthAccounts } from './oauth-accounts'
import { PasskeyManagement } from './passkey-management'
import { APITokenManagement } from './api-token-management'
import { FavoriteCitiesSettings } from './favorite-cities'
import { AlertSettings } from './alert-settings'
import { NotificationSettings } from './notification-settings'
import { ReplyPermissionSettings } from './reply-permission-settings'
import { CalendarFeedSection, FollowsActivityFeedSection } from '@/features/collections'
import { useAutoDismissBanner } from '@/lib/hooks/common'

// How long the "copied ✓" confirmation stays up after copying the CLI token.
const TOKEN_COPIED_DISMISS_MS = 2000

/** Where a reader whose session died mid-resend is sent to get a new one. */
const SIGN_IN_HREF = buildAuthHref('/profile?tab=settings')

/**
 * Settings tab, board J card order (PSY-1414 / PSY-1508), with Alerts +
 * Your area inserted by PSY-1905:
 * Account → Favorite cities → Alerts → Your area → Account emails → Calendar
 * feed (PSY-1430) → Follows activity feed (PSY-1505) → Default reply
 * permission → Connected accounts → Passkeys → Change password → API tokens →
 * CLI authentication → Export → Danger zone.
 */
export function SettingsPanel() {
  const { user } = useAuthContext()
  const exportData = useExportData()
  const generateCLIToken = useGenerateCLIToken()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [cliToken, setCLIToken] = useState<string | null>(null)
  // Shared auto-dismiss primitive rather than a hand-rolled timer, which must
  // not outlive unmount. See useAutoDismissBanner / useDismissTimer (PSY-1664).
  const {
    value: tokenCopied,
    show: showTokenCopied,
    clear: clearTokenCopied,
  } = useAutoDismissBanner<true>(TOKEN_COPIED_DISMISS_MS)

  const handleExportData = async () => {
    try {
      const data = await exportData.mutateAsync()
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: 'application/json',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `psychic-homily-export-${new Date().toISOString().split('T')[0]}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'settings', error_type: 'data_export' },
      })
    }
  }

  const handleGenerateCLIToken = async () => {
    try {
      const response = await generateCLIToken.mutateAsync()
      setCLIToken(response.token ?? null)
      clearTokenCopied()
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'settings', error_type: 'cli_token' },
      })
    }
  }

  const handleCopyToken = async () => {
    if (cliToken) {
      await navigator.clipboard.writeText(cliToken)
      showTokenCopied(true)
    }
  }

  const isEmailVerified = user?.email_verified || user?.is_admin

  return (
    <div className="space-y-6">
      {/* Account — email + verification fold (moved from Profile tab) */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Account</CardTitle>
          <CardDescription>
            Your sign-in email. Verification unlocks contributions.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm font-semibold">Email</span>
            <span className="font-mono text-[13px]">{user?.email}</span>
            {isEmailVerified ? (
              <Badge variant="outline" className="uppercase">
                Verified
              </Badge>
            ) : (
              <Badge className="uppercase">Unverified</Badge>
            )}
          </div>

          {!isEmailVerified && (
            <VerificationResend service="settings" signInHref={SIGN_IN_HREF}>
              <div className="flex flex-wrap items-center gap-2.5">
                <VerificationResendButton variant="outline" size="sm">
                  Resend verification
                </VerificationResendButton>

                {/* Compact wording by design: this row sits in a dense settings
                    column, not on a dedicated landing surface. */}
                <VerificationResendStatus
                  density="compact"
                  className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground"
                />

                <VerificationResendAlerts />
              </div>
            </VerificationResend>
          )}

          {user?.is_admin && (
            <p className="text-xs text-muted-foreground">
              Admin accounts can contribute without email verification.
            </p>
          )}
        </CardContent>
      </Card>

      <FavoriteCitiesSettings />

      {/* Alerts + Your area (PSY-1905). Sits ahead of Account emails because
          it is the card people come here for, and because the reminder and
          digest rows that used to live in that card moved into this matrix. */}
      <AlertSettings />

      <NotificationSettings />

      {/* Saved-shows iCal feed (PSY-1430) — not on board J; kept after Notifications */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Calendar feed</CardTitle>
          <CardDescription>
            Subscribe your saved shows in Google Calendar or Apple Calendar
          </CardDescription>
        </CardHeader>
        <CardContent>
          <CalendarFeedSection variant="settings" />
        </CardContent>
      </Card>

      {/* Followed-artist Atom activity feed (PSY-1505) — same personal feed token */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Follows activity feed</CardTitle>
          <CardDescription>
            Atom feed of new shows and releases for artists you follow
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FollowsActivityFeedSection />
        </CardContent>
      </Card>

      <ReplyPermissionSettings />

      <OAuthAccounts />

      <PasskeyManagement />

      <ChangePassword />

      {user?.is_admin && <APITokenManagement />}

      {user?.is_admin && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">CLI authentication</CardTitle>
            <CardDescription>
              Generate a short-lived token for the ph command-line tool.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {cliToken ? (
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <code className="flex-1 rounded-md border bg-muted/30 p-3 font-mono text-xs break-all">
                      {cliToken}
                    </code>
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={handleCopyToken}
                      className="shrink-0"
                    >
                      {tokenCopied ? (
                        <Check className="h-4 w-4 text-success-foreground" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    <AlertCircle className="h-3 w-3 shrink-0" />
                    This token expires in 24 hours. Copy it now — it won&apos;t be shown again.
                  </p>
                  <Button
                    onClick={handleGenerateCLIToken}
                    disabled={generateCLIToken.isPending}
                    variant="outline"
                    size="sm"
                  >
                    {generateCLIToken.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Terminal className="h-4 w-4" />
                    )}
                    Generate new token
                  </Button>
                </div>
              ) : (
                <Button
                  onClick={handleGenerateCLIToken}
                  disabled={generateCLIToken.isPending}
                  variant="outline"
                  size="sm"
                >
                  {generateCLIToken.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Terminal className="h-4 w-4" />
                  )}
                  Generate CLI token
                </Button>
              )}

              {generateCLIToken.isError && (
                <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span>
                    {generateCLIToken.error?.message ||
                      'Failed to generate token. Please try again.'}
                  </span>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Export your data</CardTitle>
          <CardDescription>
            Download everything tied to your account — profile, contributions,
            collections, saved shows — as JSON.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <Button
            onClick={handleExportData}
            disabled={exportData.isPending}
            variant="outline"
            size="sm"
          >
            {exportData.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Download className="h-4 w-4" />
            )}
            Export JSON
          </Button>

          {exportData.isError && (
            <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>
                {exportData.error?.message || 'Failed to export data. Please try again.'}
              </span>
            </div>
          )}

          {exportData.isSuccess && (
            <div className="flex items-center gap-2 text-sm text-success-foreground">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>Data exported successfully! Check your downloads folder.</span>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-base text-destructive">Danger zone</CardTitle>
          <CardDescription>
            Deleting your account removes your profile and sign-in. Attributed
            contributions remain, re-attributed to &ldquo;Deleted user&rdquo;.
            Recoverable for 30 days.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setDeleteDialogOpen(true)}
          >
            <AlertTriangle className="h-4 w-4" />
            Delete account
          </Button>
        </CardContent>
      </Card>

      <DeleteAccountDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
      />
    </div>
  )
}
