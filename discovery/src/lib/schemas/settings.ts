import { z } from 'zod'

export const tokenSchema = z
  .string()
  .min(1, 'Token is required')
  .regex(/^phk_/, 'Token must start with "phk_"')
  .min(20, 'Token appears to be too short')

export const urlSchema = z
  .string()
  .url('Must be a valid URL')
  .regex(/^https?:\/\//, 'Must start with http:// or https://')

export const settingsSchema = z.object({
  apiToken: z.string().optional(),
  stageToken: z.string(),
  productionToken: z.string(),
  localToken: z.string(),
  targetEnvironment: z.enum(['stage', 'production']),
  stageUrl: urlSchema,
  productionUrl: urlSchema,
})

export type SettingsFormData = z.infer<typeof settingsSchema>

export function validateToken(token: string): { valid: boolean; error?: string } {
  if (!token) {
    return { valid: true } // Empty is OK, just not configured
  }

  if (!token.startsWith('phk_')) {
    return { valid: false, error: 'Token must start with "phk_"' }
  }

  if (token.length < 20) {
    return { valid: false, error: 'Token appears to be too short' }
  }

  return { valid: true }
}
