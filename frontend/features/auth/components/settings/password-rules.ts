import { z } from 'zod'

// The same bounds as the auth page and the backend's password validator.
const MIN_PASSWORD_LENGTH = 12
const MAX_PASSWORD_LENGTH = 128

/** The rule a new password meets on the Settings password forms. */
export const newPasswordRule = z
  .string()
  .min(MIN_PASSWORD_LENGTH, `Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
  .max(MAX_PASSWORD_LENGTH, `Password must be no more than ${MAX_PASSWORD_LENGTH} characters`)

/**
 * A new password and its confirmation, for a form that asks for no current
 * password: Set a password, and the emailed reset.
 */
export const newPasswordWithConfirmSchema = z
  .object({
    newPassword: newPasswordRule,
    confirmPassword: z.string().min(1, 'Please confirm your new password'),
  })
  .refine(data => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })
