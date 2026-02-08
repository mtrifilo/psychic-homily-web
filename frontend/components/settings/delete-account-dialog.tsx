'use client'

import { useState, useEffect } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useRouter } from 'next/navigation'
import {
  AlertTriangle,
  Loader2,
  Eye,
  EyeOff,
  Lock,
  CheckCircle2,
  Calendar,
  Bookmark,
  Key,
  ArrowLeft,
  ArrowRight,
} from 'lucide-react'
import { useDeletionSummary, useDeleteAccount } from '@/lib/hooks/useAuth'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'

interface DeleteAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type Step = 'warning' | 'confirm' | 'success'

export function DeleteAccountDialog({
  open,
  onOpenChange,
}: DeleteAccountDialogProps) {
  const router = useRouter()
  const [step, setStep] = useState<Step>('warning')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [reason, setReason] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [deletionDate, setDeletionDate] = useState<string | null>(null)

  const deletionSummary = useDeletionSummary()
  const deleteAccount = useDeleteAccount()

  // Fetch deletion summary when dialog opens
  useEffect(() => {
    if (open && step === 'warning') {
      deletionSummary.refetch()
    }
  }, [open, step])

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setStep('warning')
      setPassword('')
      setShowPassword(false)
      setReason('')
      setConfirmed(false)
      setDeletionDate(null)
      deleteAccount.reset()
    }
  }, [open])

  // Auto-redirect after successful deletion
  useEffect(() => {
    if (step === 'success') {
      const timer = setTimeout(() => {
        onOpenChange(false)
        router.push('/')
      }, 3000)
      return () => clearTimeout(timer)
    }
  }, [step, router, onOpenChange])

  const handleDelete = async () => {
    try {
      const result = await deleteAccount.mutateAsync({
        password,
        reason: reason || undefined,
      })
      if (result.success && result.deletion_date) {
        setDeletionDate(result.deletion_date)
        setStep('success')
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'warning',
        tags: { service: 'account-deletion' },
      })
    }
  }

  const summary = deletionSummary.data
  const hasPassword = summary?.has_password ?? true

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {step === 'warning' && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                Delete Account
              </DialogTitle>
              <DialogDescription>
                This action will schedule your account for permanent deletion.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-4">
              {/* Data Summary */}
              {deletionSummary.isLoading ? (
                <div className="flex items-center justify-center py-4">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              ) : deletionSummary.isError ? (
                <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                  Failed to load account data. Please try again.
                </div>
              ) : (
                <>
                  <div className="rounded-lg border border-border/50 bg-muted/30 p-4">
                    <h4 className="mb-3 text-sm font-medium">
                      The following data will be affected:
                    </h4>
                    <ul className="space-y-2 text-sm text-muted-foreground">
                      <li className="flex items-center gap-2">
                        <Calendar className="h-4 w-4 shrink-0" />
                        <span>
                          <strong className="text-foreground">
                            {summary?.shows_count ?? 0}
                          </strong>{' '}
                          shows you submitted
                        </span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Bookmark className="h-4 w-4 shrink-0" />
                        <span>
                          <strong className="text-foreground">
                            {summary?.saved_shows_count ?? 0}
                          </strong>{' '}
                          saved shows
                        </span>
                      </li>
                      <li className="flex items-center gap-2">
                        <Key className="h-4 w-4 shrink-0" />
                        <span>
                          <strong className="text-foreground">
                            {summary?.passkeys_count ?? 0}
                          </strong>{' '}
                          passkeys
                        </span>
                      </li>
                    </ul>
                  </div>

                  <div className="rounded-lg border border-amber-500/20 bg-amber-500/5 p-4">
                    <div className="flex items-start gap-3">
                      <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" />
                      <div className="space-y-1 text-sm">
                        <p className="font-medium text-foreground">
                          30-Day Grace Period
                        </p>
                        <p className="text-muted-foreground">
                          Your account will be deactivated immediately, but not
                          permanently deleted for 30 days. You can recover your
                          account by contacting support during this period.
                        </p>
                      </div>
                    </div>
                  </div>
                </>
              )}
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => setStep('confirm')}
                disabled={deletionSummary.isLoading || deletionSummary.isError}
              >
                Continue
                <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </DialogFooter>
          </>
        )}

        {step === 'confirm' && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                Confirm Account Deletion
              </DialogTitle>
              <DialogDescription>
                {hasPassword
                  ? 'Enter your password to confirm deletion.'
                  : 'Confirm that you want to delete your account.'}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-4">
              {/* OAuth-only user notice */}
              {!hasPassword && (
                <div className="rounded-md bg-amber-500/10 p-3 text-sm text-amber-600 dark:text-amber-400">
                  OAuth accounts require email confirmation for deletion. This
                  feature is coming soon. Please contact support to delete your
                  account.
                </div>
              )}

              {/* Password Input */}
              {hasPassword && (
                <div className="space-y-2">
                  <Label htmlFor="delete-password">Password</Label>
                  <div className="relative">
                    <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      id="delete-password"
                      type={showPassword ? 'text' : 'password'}
                      placeholder="Enter your password"
                      value={password}
                      onChange={e => setPassword(e.target.value)}
                      className="pl-10 pr-10"
                      disabled={deleteAccount.isPending}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                      aria-label={showPassword ? 'Hide password' : 'Show password'}
                    >
                      {showPassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </button>
                  </div>
                </div>
              )}

              {/* Optional Reason */}
              <div className="space-y-2">
                <Label htmlFor="delete-reason">
                  Why are you leaving? (optional)
                </Label>
                <Textarea
                  id="delete-reason"
                  placeholder="Your feedback helps us improve..."
                  value={reason}
                  onChange={e => setReason(e.target.value)}
                  rows={3}
                  disabled={deleteAccount.isPending}
                  className="resize-none"
                />
              </div>

              {/* Confirmation Checkbox */}
              <div className="flex items-start space-x-3">
                <Checkbox
                  id="confirm-delete"
                  checked={confirmed}
                  onCheckedChange={checked => setConfirmed(checked === true)}
                  disabled={deleteAccount.isPending}
                />
                <Label
                  htmlFor="confirm-delete"
                  className="text-sm leading-relaxed text-muted-foreground"
                >
                  I understand that my account will be deactivated and scheduled
                  for permanent deletion after 30 days.
                </Label>
              </div>

              {/* Error Message */}
              {deleteAccount.isError && (
                <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                  {deleteAccount.error?.message ||
                    'Failed to delete account. Please try again.'}
                </div>
              )}
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                onClick={() => setStep('warning')}
                disabled={deleteAccount.isPending}
              >
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={
                  !confirmed ||
                  (hasPassword && !password) ||
                  !hasPassword ||
                  deleteAccount.isPending
                }
              >
                {deleteAccount.isPending ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Deleting...
                  </>
                ) : (
                  'Delete My Account'
                )}
              </Button>
            </DialogFooter>
          </>
        )}

        {step === 'success' && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-5 w-5" />
                Account Scheduled for Deletion
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4">
                <p className="text-sm text-foreground">
                  Your account has been deactivated and will be permanently
                  deleted on{' '}
                  <strong>
                    {deletionDate
                      ? new Date(deletionDate).toLocaleDateString('en-US', {
                          year: 'numeric',
                          month: 'long',
                          day: 'numeric',
                        })
                      : '30 days from now'}
                  </strong>
                  .
                </p>
                <p className="mt-2 text-sm text-muted-foreground">
                  If you change your mind, contact support to recover your
                  account before the deletion date.
                </p>
              </div>

              <p className="text-center text-sm text-muted-foreground">
                Redirecting to home page...
              </p>
            </div>

            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  onOpenChange(false)
                  router.push('/')
                }}
              >
                Go to Home
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
