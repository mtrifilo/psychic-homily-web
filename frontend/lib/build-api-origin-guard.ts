import { PHASE_PRODUCTION_BUILD } from 'next/constants'

const STAGE_API_ORIGIN = 'https://stage.api.psychichomily.com'

type BuildEnv = Readonly<Record<string, string | undefined>>

/**
 * Refuses a production build that would prerender against the production API
 * without anyone choosing it.
 *
 * With `NEXT_PUBLIC_API_URL` unset, `getApiBaseUrl` in ./api-base.ts resolves a
 * production build to the production API origin, and prerender fetches through
 * it. Vercel always sets the variable, and `VERCEL` marks a Vercel build, so a
 * Vercel build passes. Any other `next build` has to name its API origin.
 *
 * Only `PHASE_PRODUCTION_BUILD` is checked: `next dev` resolves to localhost and
 * `next start` serves an existing build. An empty value counts as unset, which
 * is how `getApiBaseUrl` reads it too.
 */
export function assertBuildApiOriginConfigured(phase: string, env: BuildEnv): void {
  if (phase !== PHASE_PRODUCTION_BUILD) return
  if (env.VERCEL) return
  if (env.NEXT_PUBLIC_API_URL) return
  throw new Error(
    `NEXT_PUBLIC_API_URL is not set, so this production build would prerender ` +
      `against the production API. Set it (and BACKEND_URL) to ` +
      `${STAGE_API_ORIGIN} or a local backend before building. ` +
      `Builds with VERCEL set skip this check.`,
  )
}
