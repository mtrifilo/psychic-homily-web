'use client'

import { useState } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react'
import { useChangePassword } from '@/features/auth'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PasswordStrengthMeter } from '@/components/ui/password-strength-meter'
import { getUniqueErrors } from '@/lib/utils/formErrors'
import { formatPasswordConfirmError } from './password-confirm-errors'
import { PasswordInput } from './password-input'
import { newPasswordRule } from './password-rules'

// Validation schema for password change
const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, 'Current password is required'),
    newPassword: newPasswordRule,
    confirmPassword: z.string().min(1, 'Please confirm your new password'),
  })
  .refine(data => data.newPassword !== data.currentPassword, {
    message: 'New password must be different from current password',
    path: ['newPassword'],
  })
  .refine(data => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type ChangePasswordFormData = z.infer<typeof changePasswordSchema>

/**
 * This form's spelling of the shared password-confirm copy. The throttle
 * sentence is the shared one; only the fallback is this form's own.
 */
export function formatChangePasswordError(error: unknown): string {
  return formatPasswordConfirmError(error, {
    fallback: 'Failed to change password',
  })
}

interface ChangePasswordProps {
  /**
   * A success line raised before this card mounted (Set a password hands its
   * confirmation over when the card swaps). It seeds the success slot, which
   * the next submit clears. Read at mount only.
   */
  notice?: string | null
}

export function ChangePassword({ notice = null }: ChangePasswordProps) {
  const changePasswordMutation = useChangePassword()
  const [newPasswordValue, setNewPasswordValue] = useState('')
  const [confirmPasswordValue, setConfirmPasswordValue] = useState('')
  const [successMessage, setSuccessMessage] = useState<string | null>(notice)

  const form = useForm({
    defaultValues: {
      currentPassword: '',
      newPassword: '',
      confirmPassword: '',
    } as ChangePasswordFormData,
    onSubmit: async ({ value }) => {
      setSuccessMessage(null)
      changePasswordMutation.mutate(
        {
          current_password: value.currentPassword,
          new_password: value.newPassword,
        },
        {
          onSuccess: data => {
            setSuccessMessage(data.message)
            // Reset the form
            form.reset()
            setNewPasswordValue('')
            setConfirmPasswordValue('')
          },
        }
      )
    },
    validators: {
      onChange: changePasswordSchema,
      onSubmit: changePasswordSchema,
    },
  })

  // Check if passwords match for real-time feedback
  const passwordsMatch = newPasswordValue === confirmPasswordValue && confirmPasswordValue.length > 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Change password</CardTitle>
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
          {/* Success message */}
          {successMessage && (
            <div role="status" className="flex items-center gap-2 rounded-md bg-success p-3 text-sm text-success-foreground">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>{successMessage}</span>
            </div>
          )}

          {/* Error message from mutation */}
          {changePasswordMutation.isError && (
            <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{formatChangePasswordError(changePasswordMutation.error)}</span>
            </div>
          )}

          {/* Current Password */}
          <form.Field name="currentPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor={field.name}>Current password</Label>
                <PasswordInput
                  id={field.name}
                  name={field.name}
                  placeholder="Enter your current password"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={e => field.handleChange(e.target.value)}
                  invalid={field.state.meta.errors.length > 0}
                />
                {field.state.meta.errors.length > 0 && (
                  <p role="alert" className="text-sm text-destructive">
                    {getUniqueErrors(field.state.meta.errors)}
                  </p>
                )}
              </div>
            )}
          </form.Field>

          {/* New Password */}
          <form.Field name="newPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor={field.name}>New password</Label>
                <PasswordInput
                  id={field.name}
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
                {/* Password strength meter */}
                <PasswordStrengthMeter password={newPasswordValue} showRequirements={true} />
                {field.state.meta.errors.length > 0 && (
                  <p role="alert" className="text-sm text-destructive">
                    {getUniqueErrors(field.state.meta.errors)}
                  </p>
                )}
              </div>
            )}
          </form.Field>

          {/* Confirm New Password */}
          <form.Field name="confirmPassword">
            {field => (
              <div className="space-y-2">
                <Label htmlFor={field.name}>Confirm new password</Label>
                <PasswordInput
                  id={field.name}
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
                {/* Password match indicator */}
                {confirmPasswordValue && field.state.meta.errors.length === 0 && (
                  <p className={`text-xs ${passwordsMatch ? 'text-success-foreground' : 'text-muted-foreground'}`}>
                    {passwordsMatch ? 'Passwords match' : 'Passwords do not match'}
                  </p>
                )}
              </div>
            )}
          </form.Field>

          {/* Submit button */}
          <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
            {([canSubmit, isSubmitting]) => (
              <Button
                type="submit"
                disabled={!canSubmit || isSubmitting || changePasswordMutation.isPending}
                className="w-full sm:w-auto"
              >
                {isSubmitting || changePasswordMutation.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Changing password...
                  </>
                ) : (
                  'Update password'
                )}
              </Button>
            )}
          </form.Subscribe>
        </form>
      </CardContent>
    </Card>
  )
}
