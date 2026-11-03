'use client'

import { Suspense, useEffect, useRef, useState, type ReactNode } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2, ServerCrash, User } from 'lucide-react'
import { useCheckPasswordReset, useCompletePasswordReset } from '@/features/auth'
import { AccountRecoverableMessage } from '@/app/auth/_components/account-recoverable-message'
import { PasswordInput } from '@/features/auth/components/settings/password-input'
import { newPasswordRule } from '@/features/auth/components/settings/password-rules'
import { formatPasswordConfirmError } from '@/features/auth/components/settings/password-confirm-errors'
import { SESSIONS_SETTINGS_HREF } from '@/features/auth/components/settings/sessions'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  AuthErrorCode,
  isAccountInactive,
  isAccountRecoverable,
  isAuthError,
} from '@/lib/errors'
import { ACCOUNT_UNAVAILABLE_MESSAGE } from '@/lib/errors/authRefusalCopy'
import { PASSWORD_RESET_EXPIRY_MINUTES } from '@/lib/password-reset'
import { getUniqueErrors } from '@/lib/utils/formErrors'
import { Button } from '@/components/ui/button'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { PasswordStrengthMeter } from '@/components/ui/password-strength-meter'

const resetPasswordSchema = z
  .object({
    newPassword: newPasswordRule,
    confirmPassword: z.string().min(1, 'Please confirm your new password'),
  })
  .refine(data => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type ResetPasswordFormData = z.infer<typeof resetPasswordSchema>

/** Where "Request a new link" and "Sign in" go: the /auth Sign in tab. */
const SIGN_IN_HREF = '/auth'

const KICKER_CLASS = 'font-data text-[11px] uppercase tracking-[0.66px] text-muted-foreground'

/**
 * A 5xx or a transport failure: the link was not judged, so a new one would
 * not help and the same one is retried. A body-encoded refusal carries a
 * sub-500 status (useCheckPasswordReset, useCompletePasswordReset).
 */
function isServerSideFailure(error: unknown): boolean {
  const status = (error as { status?: number } | null)?.status
  return typeof status !== 'number' || status >= 500
}

/** The link is malformed, expired, used, or no longer the account's. */
function isLinkRefusal(error: unknown): boolean {
  return isAuthError(error) && error.code === AuthErrorCode.INVALID_TOKEN
}

function CenteredCard({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">{children}</div>
    </div>
  )
}

/** A card that replaces the page's previous state, its heading taking focus. */
function StateCard({
  kicker,
  title,
  children,
}: {
  kicker: string
  title: string
  children: ReactNode
}) {
  return (
    <CenteredCard>
      <Card className="gap-4 px-10 py-9">
        <p className={KICKER_CLASS}>{kicker}</p>
        <h1
          ref={heading => heading?.focus()}
          tabIndex={-1}
          className="font-display text-[26px] font-bold leading-tight text-foreground focus:outline-none"
        >
          {title}
        </h1>
        {children}
      </Card>
    </CenteredCard>
  )
}

/** State 5: an expired, used or incomplete link. The password is untouched. */
function LinkNoLongerWorks() {
  return (
    <StateCard kicker="Password reset · link no longer works" title="That link no longer works.">
      <p className="text-sm leading-[22px] text-foreground">
        Reset links work once and expire {PASSWORD_RESET_EXPIRY_MINUTES} minutes after they are
        sent. Your password has not changed.
      </p>
      <div>
        <Button asChild>
          <Link href={SIGN_IN_HREF}>Request a new link</Link>
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">
        Already set a new password?{' '}
        <Link href={SIGN_IN_HREF} className="underline underline-offset-4 hover:text-primary">
          Sign in
        </Link>
        .
      </p>
    </StateCard>
  )
}

/** State 4A: the password is set, this browser is signed in, the rest are not. */
function PasswordResetDone() {
  return (
    <StateCard kicker="Password reset · signed in" title="New password set.">
      <p className="text-sm leading-[22px] text-foreground">
        You&apos;re signed in on this device. Every other device was signed out. Your passkeys
        still work.
      </p>
      <div className="flex flex-wrap items-center gap-3">
        <Button asChild>
          <Link href="/">Continue</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href={SESSIONS_SETTINGS_HREF}>Review signed-in devices</Link>
        </Button>
      </div>
    </StateCard>
  )
}

/** An account that may no longer sign in, in the magic-link page's words. */
function AccountClosed({ error }: { error: unknown }) {
  return (
    <CenteredCard>
      <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
            <AlertCircle className="h-6 w-6 text-destructive" />
          </div>
          <CardTitle>Can&apos;t sign you in</CardTitle>
          <CardDescription>
            {isAccountRecoverable(error) ? <AccountRecoverableMessage /> : ACCOUNT_UNAVAILABLE_MESSAGE}
          </CardDescription>
        </CardHeader>
      </Card>
    </CenteredCard>
  )
}

/** The link could not be checked; it is still unspent, so the same one is retried. */
function CheckFailed({ onRetry }: { onRetry: () => void }) {
  return (
    <CenteredCard>
      <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
            <ServerCrash className="h-6 w-6 text-destructive" />
          </div>
          <CardTitle>Something went wrong</CardTitle>
          <CardDescription>
            We couldn&apos;t open your link because of a problem on our end. Your link still
            works. Please try again in a moment.
          </CardDescription>
        </CardHeader>
        <div className="flex justify-center px-6 pb-2">
          <Button onClick={onRetry}>Try Again</Button>
        </div>
      </Card>
    </CenteredCard>
  )
}

function Checking() {
  return (
    <CenteredCard>
      <div className="flex justify-center py-12" role="status" aria-label="Checking your link">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    </CenteredCard>
  )
}

/** State 3: a new password and its confirmation, under the Settings rules. */
function ResetPasswordForm({
  email,
  submitting,
  error,
  onSubmit,
}: {
  email: string
  submitting: boolean
  /** The latest refusal or failure of a submit, shown above the fields. */
  error: unknown
  onSubmit: (newPassword: string) => void
}) {
  const [newPasswordValue, setNewPasswordValue] = useState('')
  const [confirmPasswordValue, setConfirmPasswordValue] = useState('')

  const form = useForm({
    defaultValues: { newPassword: '', confirmPassword: '' } as ResetPasswordFormData,
    onSubmit: async ({ value }) => onSubmit(value.newPassword),
    validators: {
      onChange: resetPasswordSchema,
      onSubmit: resetPasswordSchema,
    },
  })

  const passwordsMatch = newPasswordValue === confirmPasswordValue && confirmPasswordValue.length > 0

  return (
    <CenteredCard>
      <div className="mb-8 text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <User className="h-6 w-6 text-primary" />
        </div>
        <h1 className="text-2xl font-bold tracking-tight">Reset your password</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          For <span className="font-data break-words">{email}</span>
        </p>
      </div>

      <Card className="border-border/50 bg-card/50 px-6 py-6 backdrop-blur-sm">
        <form
          onSubmit={e => {
            e.preventDefault()
            e.stopPropagation()
            form.handleSubmit()
          }}
          className="space-y-4"
        >
          {error ? (
            <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{formatPasswordConfirmError(error, { fallback: 'Failed to reset password' })}</span>
            </div>
          ) : null}

          <form.Field name="newPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor="reset-password-new">New password</Label>
                <PasswordInput
                  id="reset-password-new"
                  name={field.name}
                  autoFocus
                  placeholder="At least 12 characters"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={e => {
                    field.handleChange(e.target.value)
                    setNewPasswordValue(e.target.value)
                  }}
                  invalid={field.state.meta.errors.length > 0}
                />
                <PasswordStrengthMeter password={newPasswordValue} showRequirements={true} />
                {field.state.meta.errors.length > 0 && (
                  <p role="alert" className="text-sm text-destructive">
                    {getUniqueErrors(field.state.meta.errors)}
                  </p>
                )}
              </div>
            )}
          </form.Field>

          <form.Field name="confirmPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor="reset-password-confirm">Confirm password</Label>
                <PasswordInput
                  id="reset-password-confirm"
                  name={field.name}
                  placeholder="Confirm your new password"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={e => {
                    field.handleChange(e.target.value)
                    setConfirmPasswordValue(e.target.value)
                  }}
                  invalid={field.state.meta.errors.length > 0}
                />
                {field.state.meta.errors.length > 0 && (
                  <p role="alert" className="text-sm text-destructive">
                    {getUniqueErrors(field.state.meta.errors)}
                  </p>
                )}
                {confirmPasswordValue && field.state.meta.errors.length === 0 && (
                  <p className={`text-xs ${passwordsMatch ? 'text-success-foreground' : 'text-muted-foreground'}`}>
                    {passwordsMatch ? 'Passwords match' : 'Passwords do not match'}
                  </p>
                )}
              </div>
            )}
          </form.Field>

          <div className="space-y-1.5 border border-border bg-background px-4 py-3">
            <p className="font-data text-[11px] uppercase tracking-[0.55px] text-muted-foreground">
              When you set it
            </p>
            <p className="text-[13px] leading-5 text-foreground">
              Every other device signed in to this account is signed out. Your passkeys keep
              working.
            </p>
          </div>

          <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
            {([canSubmit, isSubmitting]) => (
              <Button
                type="submit"
                className="w-full"
                disabled={!canSubmit || isSubmitting || submitting}
              >
                {(isSubmitting || submitting) && <Loader2 className="h-4 w-4 animate-spin" />}
                Set new password
              </Button>
            )}
          </form.Subscribe>

          <div className="flex justify-center">
            <Link
              href={SIGN_IN_HREF}
              className="text-sm text-muted-foreground underline underline-offset-4 hover:text-primary"
            >
              Cancel and go back to sign in
            </Link>
          </div>
        </form>
      </Card>
    </CenteredCard>
  )
}

/**
 * The page an emailed reset link opens. Opening it checks the link and spends
 * nothing; only a successful set does, so Cancel, a reload, or a mail scanner
 * fetching the link leaves it usable until it expires.
 */
function ResetPasswordContent() {
  const token = useSearchParams().get('token')
  const { setUser } = useAuthContext()
  const check = useCheckPasswordReset()
  const complete = useCompletePasswordReset()
  const checkedTokenRef = useRef<string | null>(null)

  // Check once per token value.
  useEffect(() => {
    if (!token || checkedTokenRef.current === token) return
    checkedTokenRef.current = token
    check.mutate(token)
  }, [token, check])

  if (!token) return <LinkNoLongerWorks />
  if (complete.isSuccess) return <PasswordResetDone />

  // A submit's refusal of the link or the account outranks the check's
  // earlier answer: the link may have been used, or the account closed, since.
  for (const error of [complete.error, check.error]) {
    if (isLinkRefusal(error)) return <LinkNoLongerWorks />
    if (isAccountInactive(error) || isAccountRecoverable(error)) return <AccountClosed error={error} />
  }

  if (check.isError) {
    if (isServerSideFailure(check.error)) {
      return (
        <CheckFailed
          onRetry={() => {
            checkedTokenRef.current = null
            check.reset()
          }}
        />
      )
    }
    return <LinkNoLongerWorks />
  }
  if (!check.isSuccess || !check.data.email) return <Checking />

  return (
    <ResetPasswordForm
      email={check.data.email}
      submitting={complete.isPending}
      error={complete.error}
      onSubmit={newPassword =>
        complete.mutate(
          { token, newPassword },
          {
            onSuccess: data => {
              if (data.user) setUser(data.user)
            },
          }
        )
      }
    />
  )
}

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<Checking />}>
      <ResetPasswordContent />
    </Suspense>
  )
}
