'use client'

import { useState, type FormEvent } from 'react'
import { useRouter } from 'next/navigation'
import { AlertCircle, CheckCircle2, Info, Loader2 } from 'lucide-react'
import { useLogin } from '@/features/auth'
import {
  SEND_LINK_FAILED_MESSAGE,
  useSendOAuthLinkEmail,
} from '@/features/auth/hooks/useOAuthLink'
import { useAuthContext } from '@/lib/context/AuthContext'
import { AuthErrorCode, isAuthError } from '@/lib/errors'
import { MAGIC_LINK_EXPIRY_MINUTES } from '@/lib/magic-link'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { googleStartUrl } from './google-start-url'

/**
 * The plain line the Sign in tab shows when the pending link is gone
 * (expired, used, or not this browser's): the sign-in refusal's own words,
 * oauthLinkRefusedMessage in backend/internal/errors/auth.go, since with no
 * pending link left that refusal's remedy is the way to connect Google.
 */
export const PENDING_LINK_GONE_MESSAGE =
  'An account already uses this email address. Sign in to that account first, then connect this provider in Settings under Connected accounts.'

const PASSWORD_REQUIRED_MESSAGE = 'Password is required'

const LINK_CLASS = 'font-medium underline underline-offset-4 hover:text-primary'

interface GoogleCollisionSignInProps {
  /** The address the account and the Google identity share. */
  email: string
  /** Where the reader lands after signing in, already through sanitizeReturnTo. */
  returnTo: string
  /** The pending link turned out to be gone; the page falls back to the normal tab. */
  onPendingLinkGone: () => void
}

/**
 * The Sign in tab's collision state: the Google sign-in's verified address
 * belongs to an account, and the person proves it holds that account by its
 * password or by its mailbox. Google, passkey and "Forgot password?" are left
 * out, so those two are the only proofs offered.
 */
export function GoogleCollisionSignIn({
  email,
  returnTo,
  onPendingLinkGone,
}: GoogleCollisionSignInProps) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  const login = useLogin()
  const sendLink = useSendOAuthLinkEmail()
  const [password, setPassword] = useState('')
  const [passwordMissing, setPasswordMissing] = useState(false)

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password === '') {
      setPasswordMissing(true)
      return
    }
    login.mutate(
      { email, password },
      {
        onSuccess: data => {
          if (data.user) setUser(data.user)
          router.push(returnTo)
        },
      }
    )
  }

  const onSendLink = () => {
    sendLink.mutate(undefined, {
      onError: error => {
        if (isAuthError(error) && error.code === AuthErrorCode.OAUTH_LINK_PENDING_EXPIRED) {
          onPendingLinkGone()
        }
      },
    })
  }

  const sendError =
    sendLink.isError &&
    !(isAuthError(sendLink.error) && sendLink.error.code === AuthErrorCode.OAUTH_LINK_PENDING_EXPIRED)

  return (
    <div className="space-y-4">
      {/* A statement of the state, not an interruption: a note, not an alert. */}
      <Alert role="note">
        <Info className="h-4 w-4" />
        <AlertDescription>
          <span>
            An account already uses <strong className="font-semibold">{email}</strong>. Sign in
            with your password to connect Google.
          </span>
        </AlertDescription>
      </Alert>

      {login.error && (
        <Alert key={login.error.message} variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{login.error.message}</AlertDescription>
        </Alert>
      )}

      <form noValidate onSubmit={onSubmit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="collision-email">Email</Label>
          <Input
            id="collision-email"
            name="email"
            type="email"
            value={email}
            readOnly
            className="bg-muted font-mono text-sm"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="collision-password">Password</Label>
          <Input
            id="collision-password"
            name="password"
            type="password"
            autoComplete="current-password"
            placeholder="Enter your password"
            value={password}
            onChange={event => {
              setPassword(event.target.value)
              setPasswordMissing(false)
            }}
            aria-invalid={passwordMissing}
            aria-describedby={passwordMissing ? 'collision-password-error' : undefined}
          />
          {passwordMissing && (
            <p id="collision-password-error" role="alert" className="text-sm text-destructive">
              {PASSWORD_REQUIRED_MESSAGE}
            </p>
          )}
        </div>

        <Button type="submit" className="w-full" disabled={login.isPending}>
          {login.isPending ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin" />
              Signing in...
            </>
          ) : (
            'Sign in and connect Google'
          )}
        </Button>
      </form>

      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t" />
        </div>
        <div className="relative flex justify-center text-xs">
          <span className="bg-card px-2 text-muted-foreground">or</span>
        </div>
      </div>

      {sendLink.isSuccess ? (
        <div
          role="status"
          className="flex gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-400"
        >
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
          <div className="space-y-1">
            <p className="font-medium">Check your inbox</p>
            <p>
              We sent a sign-in link to <span className="font-mono">{email}</span>. It expires in{' '}
              {MAGIC_LINK_EXPIRY_MINUTES} minutes. Opening it connects Google and signs out your
              other sessions.
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-2">
          {sendError && (
            <div
              role="alert"
              className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive"
            >
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{SEND_LINK_FAILED_MESSAGE}</span>
            </div>
          )}
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={onSendLink}
            disabled={sendLink.isPending}
          >
            {sendLink.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Sending...
              </>
            ) : (
              'Email me a sign-in link instead'
            )}
          </Button>
          <p className="text-center text-xs text-muted-foreground">
            The link connects Google, signs out your other sessions and turns off your current
            password.
          </p>
        </div>
      )}

      <p className="text-center text-xs text-muted-foreground">
        Not your account?{' '}
        <a href={googleStartUrl({ returnTo, selectAccount: true })} className={LINK_CLASS}>
          Use a different Google account
        </a>
      </p>
    </div>
  )
}
