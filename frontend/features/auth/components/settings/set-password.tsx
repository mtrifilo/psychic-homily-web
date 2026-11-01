'use client'

import { useState } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2 } from 'lucide-react'
import { useSetPassword } from '@/features/auth'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PasswordStrengthMeter } from '@/components/ui/password-strength-meter'
import { CredentialErrorMessage } from '@/components/shared/CredentialErrorMessage'
import { isPasswordAlreadySet, PASSWORD_ALREADY_SET_MESSAGE } from '@/lib/errors'
import { getUniqueErrors } from '@/lib/utils/formErrors'
import { formatPasswordConfirmError } from './password-confirm-errors'
import { PasswordInput } from './password-input'
import { newPasswordRule } from './password-rules'

const setPasswordSchema = z
  .object({
    newPassword: newPasswordRule,
    confirmPassword: z.string().min(1, 'Please confirm your new password'),
  })
  .refine(data => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type SetPasswordFormData = z.infer<typeof setPasswordSchema>

/**
 * The card's opening sentence, naming the ways the account signs in today, or
 * null when it signs in some other way, which has no sentence of its own.
 */
export function passwordlessSignInDescription(
  providers: readonly string[],
  passkeyCount: number
): string | null {
  const google = providers.includes('google')
  const passkey = passkeyCount > 0
  if (google && passkey) return 'You sign in with Google or a passkey. This account has no password.'
  if (google) return 'You sign in with Google. This account has no password.'
  if (passkey) return 'You sign in with a passkey.'
  return null
}

interface SetPasswordProps {
  /** The providers the account signs in with, from the profile. */
  providers: readonly string[]
  /** The account's passkey count, from the profile. */
  passkeyCount: number
  /** Where the re-authentication link returns the reader: this card. */
  reauthReturnTo: string
  /** Called with the confirmation line once the password is set. */
  onPasswordSet: (confirmation: string) => void
}

/**
 * The Password card for an account that has no password: a prompt that opens
 * a form setting the first one. No current password is asked for, and the
 * request needs a recent sign-in, which the line under the buttons says up
 * front.
 */
export function SetPassword({
  providers,
  passkeyCount,
  reauthReturnTo,
  onPasswordSet,
}: SetPasswordProps) {
  const [open, setOpen] = useState(false)
  const description = passwordlessSignInDescription(providers, passkeyCount)

  if (!open) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Password</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm">
              Add one to also sign in with your email and password. Your other devices stay signed
              in.
            </p>
            <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
              Set a password
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <SetPasswordForm
      reauthReturnTo={reauthReturnTo}
      onCancel={() => setOpen(false)}
      onPasswordSet={onPasswordSet}
    />
  )
}

function SetPasswordForm({
  reauthReturnTo,
  onCancel,
  onPasswordSet,
}: {
  reauthReturnTo: string
  onCancel: () => void
  onPasswordSet: (confirmation: string) => void
}) {
  const setPasswordMutation = useSetPassword()
  const [newPasswordValue, setNewPasswordValue] = useState('')
  const [confirmPasswordValue, setConfirmPasswordValue] = useState('')

  const form = useForm({
    defaultValues: { newPassword: '', confirmPassword: '' } as SetPasswordFormData,
    onSubmit: async ({ value }) => {
      setPasswordMutation.mutate(value.newPassword, {
        onSuccess: () => {
          onPasswordSet('Password set. You can now also sign in with your email and password.')
        },
      })
    },
    validators: {
      onChange: setPasswordSchema,
      onSubmit: setPasswordSchema,
    },
  })

  const passwordsMatch = newPasswordValue === confirmPasswordValue && confirmPasswordValue.length > 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Set a password</CardTitle>
        <CardDescription>
          Adds email and password as a way to sign in. Your other devices stay signed in.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={e => {
            e.preventDefault()
            e.stopPropagation()
            form.handleSubmit()
          }}
          className="space-y-4"
        >
          {setPasswordMutation.isError && (
            <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>
                <SetPasswordErrorMessage error={setPasswordMutation.error} reauthReturnTo={reauthReturnTo} />
              </span>
            </div>
          )}

          <form.Field name="newPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor="set-password-new">New password</Label>
                <PasswordInput
                  id="set-password-new"
                  name={field.name}
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
                <Label htmlFor="set-password-confirm">Confirm password</Label>
                <PasswordInput
                  id="set-password-confirm"
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

          <div className="space-y-3">
            <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
              {([canSubmit, isSubmitting]) => (
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="submit"
                    disabled={!canSubmit || isSubmitting || setPasswordMutation.isPending}
                  >
                    {(isSubmitting || setPasswordMutation.isPending) && (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    )}
                    Set password
                  </Button>
                  <Button type="button" variant="outline" onClick={onCancel}>
                    Cancel
                  </Button>
                </div>
              )}
            </form.Subscribe>
            <p className="text-xs text-muted-foreground">
              If you signed in a while ago, we&apos;ll ask you to sign in again first.
            </p>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

/**
 * One line for a refused set: the re-authentication refusal with its link back
 * to this card, the already-set refusal in this card's words, and anything
 * else in the shared password-form wording (a throttle, or the server's own
 * message, such as the validator's).
 */
function SetPasswordErrorMessage({
  error,
  reauthReturnTo,
}: {
  error: unknown
  reauthReturnTo: string
}) {
  if (isPasswordAlreadySet(error)) return <>{PASSWORD_ALREADY_SET_MESSAGE}</>
  return (
    <CredentialErrorMessage
      error={error}
      returnTo={reauthReturnTo}
      fallback={formatPasswordConfirmError(error, { fallback: 'Failed to set password' })}
    />
  )
}
