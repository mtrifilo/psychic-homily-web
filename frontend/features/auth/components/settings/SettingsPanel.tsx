'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useSendVerificationEmail, useExportData, useGenerateCLIToken } from '@/features/auth'
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
import { APITokenManagement } from './api-token-management'
import { FavoriteCitiesSettings } from './favorite-cities'
import { NotificationSettings } from './notification-settings'
import { ReplyPermissionSettings } from './reply-permission-settings'
import { CalendarFeedSection } from '@/features/collections'

/**
 * Settings tab — board J card order (PSY-1414):
 * Account → Favorite cities → Notifications → Calendar feed (PSY-1430) →
 * Default reply permission → Connected accounts → (Passkeys deferred) →
 * Change password → API tokens → CLI authentication → Export → Danger zone.
 */
export function SettingsPanel() {
  const { user } = useAuthContext()
  const sendVerificationEmail = useSendVerificationEmail()
  const exportData = useExportData()
  const generateCLIToken = useGenerateCLIToken()
  const [emailSent, setEmailSent] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [cliToken, setCLIToken] = useState<string | null>(null)
  const [tokenCopied, setTokenCopied] = useState(false)

  const handleSendVerification = async () => {
    try {
      await sendVerificationEmail.mutateAsync()
      setEmailSent(true)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'settings', error_type: 'verification_email' },
      })
    }
  }

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
      setTokenCopied(false)
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
      setTokenCopied(true)
      setTimeout(() => setTokenCopied(false), 2000)
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
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-mono text-sm">{user?.email}</p>
            {isEmailVerified ? (
              <Badge variant="outline">Verified</Badge>
            ) : (
              <Badge variant="outline">Not verified</Badge>
            )}
          </div>

          {!isEmailVerified && (
            <div className="space-y-2">
              {emailSent && sendVerificationEmail.isSuccess ? (
                <div className="flex items-center gap-2 text-sm text-success-foreground">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  <span>Verification email sent! Check your inbox.</span>
                </div>
              ) : (
                <Button
                  onClick={handleSendVerification}
                  disabled={sendVerificationEmail.isPending}
                  variant="outline"
                  size="sm"
                >
                  {sendVerificationEmail.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : null}
                  Resend verification
                </Button>
              )}

              {sendVerificationEmail.isError && (
                <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span>
                    {sendVerificationEmail.error?.message ||
                      'Failed to send verification email. Please try again.'}
                  </span>
                </div>
              )}
            </div>
          )}

          {user?.is_admin && (
            <p className="text-xs text-muted-foreground">
              Admin accounts can contribute without email verification.
            </p>
          )}
        </CardContent>
      </Card>

      <FavoriteCitiesSettings />

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

      <ReplyPermissionSettings />

      <OAuthAccounts />

      {/* Passkeys deferred — PasskeyManagement exists; wire-up filed as follow-up */}

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
