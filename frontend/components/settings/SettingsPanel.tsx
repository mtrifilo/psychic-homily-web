'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useSendVerificationEmail, useExportData, useGenerateCLIToken } from '@/lib/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Mail,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Send,
  AlertTriangle,
  Download,
  FileJson,
  Terminal,
  Copy,
  Check,
} from 'lucide-react'
import { ChangePassword } from '@/components/settings/change-password'
import { DeleteAccountDialog } from '@/components/settings/delete-account-dialog'
import { OAuthAccounts } from '@/components/settings/oauth-accounts'
import { APITokenManagement } from '@/components/settings/api-token-management'
import { FavoriteCitiesSettings } from '@/components/settings/favorite-cities'

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
      // Error captured by Sentry above; UI shows error via mutation.isError
    }
  }

  const handleExportData = async () => {
    try {
      const data = await exportData.mutateAsync()
      // Create a blob from the JSON data and trigger download
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
      // Error captured by Sentry above; UI shows error via mutation.isError
    }
  }

  const handleGenerateCLIToken = async () => {
    try {
      const response = await generateCLIToken.mutateAsync()
      setCLIToken(response.token)
      setTokenCopied(false)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'settings', error_type: 'cli_token' },
      })
      // Error captured by Sentry above; UI shows error via mutation.isError
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
      {/* Favorite Cities Section */}
      <FavoriteCitiesSettings />

      {/* Email Verification Section */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Mail className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-lg">Email Verification</CardTitle>
          </div>
          <CardDescription>
            Verify your email to submit shows to the calendar
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Current Email Status */}
            <div className="flex items-center justify-between rounded-lg border border-border/50 bg-muted/30 p-4">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-background">
                  <Mail className="h-5 w-5 text-muted-foreground" />
                </div>
                <div>
                  <p className="text-sm font-medium">{user?.email}</p>
                  <p className="text-xs text-muted-foreground">
                    {user?.is_admin ? 'Admin account' : 'Your email address'}
                  </p>
                </div>
              </div>
              
              {isEmailVerified ? (
                <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-0">
                  <CheckCircle2 className="mr-1 h-3 w-3" />
                  Verified
                </Badge>
              ) : (
                <Badge variant="secondary" className="bg-amber-500/10 text-amber-600 dark:text-amber-400 border-0">
                  <AlertCircle className="mr-1 h-3 w-3" />
                  Not Verified
                </Badge>
              )}
            </div>

            {/* Verification Action */}
            {!isEmailVerified && (
              <div className="rounded-lg border border-amber-500/20 bg-amber-500/5 p-4">
                <div className="flex items-start gap-3">
                  <AlertCircle className="h-5 w-5 text-amber-500 mt-0.5 shrink-0" />
                  <div className="flex-1 space-y-3">
                    <div>
                      <p className="text-sm font-medium text-foreground">
                        Email verification required
                      </p>
                      <p className="text-sm text-muted-foreground mt-1">
                        You need to verify your email address before you can submit shows to the Arizona music calendar.
                      </p>
                    </div>

                    {emailSent && sendVerificationEmail.isSuccess ? (
                      <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
                        <CheckCircle2 className="h-4 w-4" />
                        <span>Verification email sent! Check your inbox.</span>
                      </div>
                    ) : (
                      <Button
                        onClick={handleSendVerification}
                        disabled={sendVerificationEmail.isPending}
                        className="gap-2"
                        size="sm"
                      >
                        {sendVerificationEmail.isPending ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Send className="h-4 w-4" />
                        )}
                        Send Verification Email
                      </Button>
                    )}

                    {sendVerificationEmail.isError && (
                      <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                        <AlertCircle className="h-4 w-4" />
                        <span>
                          {sendVerificationEmail.error?.message || 'Failed to send verification email. Please try again.'}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}

            {/* Verified Success State */}
            {isEmailVerified && !user?.is_admin && (
              <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4">
                <div className="flex items-center gap-3">
                  <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                  <div>
                    <p className="text-sm font-medium text-foreground">
                      Your email is verified
                    </p>
                    <p className="text-sm text-muted-foreground mt-0.5">
                      You can submit shows to the Arizona music calendar.
                    </p>
                  </div>
                </div>
              </div>
            )}

            {/* Admin notice */}
            {user?.is_admin && (
              <div className="rounded-lg border border-primary/20 bg-primary/5 p-4">
                <div className="flex items-center gap-3">
                  <CheckCircle2 className="h-5 w-5 text-primary" />
                  <div>
                    <p className="text-sm font-medium text-foreground">
                      Admin account
                    </p>
                    <p className="text-sm text-muted-foreground mt-0.5">
                      As an admin, you can submit shows without email verification.
                    </p>
                  </div>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Connected Accounts Section */}
      <OAuthAccounts />

      {/* Password Change Section - only show for users with passwords */}
      <ChangePassword />

      {/* Data Export Section (GDPR Right to Portability) */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <FileJson className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-lg">Export Your Data</CardTitle>
          </div>
          <CardDescription>
            Download a copy of all your data in JSON format
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="rounded-lg border border-border/50 bg-muted/30 p-4">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-foreground">
                    Your data export includes:
                  </p>
                  <ul className="text-sm text-muted-foreground list-disc list-inside space-y-0.5">
                    <li>Profile information</li>
                    <li>Email preferences</li>
                    <li>Connected accounts</li>
                    <li>Passkeys</li>
                    <li>Saved shows</li>
                    <li>Submitted shows</li>
                  </ul>
                </div>
                <Button
                  onClick={handleExportData}
                  disabled={exportData.isPending}
                  variant="outline"
                  className="shrink-0 gap-2"
                >
                  {exportData.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Download className="h-4 w-4" />
                  )}
                  Export My Data
                </Button>
              </div>
            </div>

            {exportData.isError && (
              <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                <AlertCircle className="h-4 w-4" />
                <span>
                  {exportData.error?.message || 'Failed to export data. Please try again.'}
                </span>
              </div>
            )}

            {exportData.isSuccess && (
              <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-4 w-4" />
                <span>Data exported successfully! Check your downloads folder.</span>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* API Token Management (Admin Only) */}
      {user?.is_admin && <APITokenManagement />}

      {/* CLI Token Section (Admin Only) - Quick 24-hour tokens */}
      {user?.is_admin && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Terminal className="h-5 w-5 text-muted-foreground" />
              <CardTitle className="text-lg">CLI Authentication</CardTitle>
            </div>
            <CardDescription>
              Generate a short-lived token for the admin CLI tool
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="rounded-lg border border-border/50 bg-muted/30 p-4">
                <div className="flex flex-col gap-4">
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-foreground">
                      Quick CLI Token (24 hours)
                    </p>
                    <p className="text-sm text-muted-foreground">
                      Generate a short-lived token for quick CLI sessions. For long-running automations, use API Tokens above.
                    </p>
                  </div>

                  {cliToken ? (
                    <div className="space-y-3">
                      <div className="flex items-center gap-2">
                        <code className="flex-1 rounded-md bg-background p-3 text-xs font-mono break-all border">
                          {cliToken}
                        </code>
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={handleCopyToken}
                          className="shrink-0"
                        >
                          {tokenCopied ? (
                            <Check className="h-4 w-4 text-emerald-500" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <AlertCircle className="h-3 w-3" />
                        <span>This token will expire in 24 hours. Copy it now — it won&apos;t be shown again.</span>
                      </div>
                      <Button
                        onClick={handleGenerateCLIToken}
                        disabled={generateCLIToken.isPending}
                        variant="outline"
                        size="sm"
                        className="gap-2"
                      >
                        {generateCLIToken.isPending ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Terminal className="h-4 w-4" />
                        )}
                        Generate New Token
                      </Button>
                    </div>
                  ) : (
                    <Button
                      onClick={handleGenerateCLIToken}
                      disabled={generateCLIToken.isPending}
                      variant="outline"
                      className="gap-2 w-fit"
                    >
                      {generateCLIToken.isPending ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Terminal className="h-4 w-4" />
                      )}
                      Generate CLI Token
                    </Button>
                  )}
                </div>
              </div>

              {generateCLIToken.isError && (
                <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4" />
                  <span>
                    {generateCLIToken.error?.message || 'Failed to generate token. Please try again.'}
                  </span>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Danger Zone - Account Deletion */}
      <Card className="border-destructive/50">
        <CardHeader>
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive" />
            <CardTitle className="text-lg text-destructive">Danger Zone</CardTitle>
          </div>
          <CardDescription>
            Irreversible actions that affect your account
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="rounded-lg border border-destructive/20 bg-destructive/5 p-4">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-foreground">
                    Delete your account
                  </p>
                  <p className="text-sm text-muted-foreground">
                    Permanently delete your account and all associated data.
                    You&apos;ll have 30 days to recover your account.
                  </p>
                </div>
                <Button
                  variant="destructive"
                  onClick={() => setDeleteDialogOpen(true)}
                  className="shrink-0"
                >
                  <AlertTriangle className="mr-2 h-4 w-4" />
                  Delete Account
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Delete Account Dialog */}
      <DeleteAccountDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
      />
    </div>
  )
}
