'use client'

import { useState } from 'react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useSendVerificationEmail } from '@/lib/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Mail,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Send,
} from 'lucide-react'
import { ChangePassword } from '@/components/settings/change-password'

export function SettingsPanel() {
  const { user } = useAuthContext()
  const sendVerificationEmail = useSendVerificationEmail()
  const [emailSent, setEmailSent] = useState(false)

  const handleSendVerification = async () => {
    try {
      await sendVerificationEmail.mutateAsync()
      setEmailSent(true)
    } catch (error) {
      // Error handling is done in the mutation
      console.error('Failed to send verification email:', error)
    }
  }

  const isEmailVerified = user?.email_verified || user?.is_admin

  return (
    <div className="space-y-6">
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
                      <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
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

      {/* Password Change Section - only show for users with passwords */}
      <ChangePassword />
    </div>
  )
}
