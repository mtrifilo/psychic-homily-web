import { z } from 'zod'

// The same bounds as the auth page and the backend's password validator.
const MIN_PASSWORD_LENGTH = 12
const MAX_PASSWORD_LENGTH = 128

/** The rule a new password meets on the Settings password forms. */
export const newPasswordRule = z
  .string()
  .min(MIN_PASSWORD_LENGTH, `Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
  .max(MAX_PASSWORD_LENGTH, `Password must be no more than ${MAX_PASSWORD_LENGTH} characters`)
