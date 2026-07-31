/**
 * Validate-only global setup for `e2e/playwright.external.config.ts`.
 *
 * It provisions NOTHING — no database, no backend, no auth state. Its whole job
 * is to refuse a run that has not been told which stack to talk to, and to
 * state the resolved targets so a run against the wrong stack is visible rather
 * than silently green.
 *
 * It exists as a global setup rather than as module-scope validation in the
 * config so that merely *loading* the config — `--list`, shard counting, an IDE
 * enumerating configs — does not throw. Playwright still runs this before the
 * first test, so the fail-fast guarantee is unchanged.
 */
function required(name: string, value: string | undefined): string {
  if (!value) {
    throw new Error(
      `${name} is required by e2e/playwright.external.config.ts. This config ` +
        `never provisions a stack — it only talks to one that is already ` +
        `running, so it cannot guess where that is. Set BACKEND_URL and ` +
        `E2E_BASE_URL (or STACK_FRONTEND_URL) to the SAME stack. They are ` +
        `separate variables with no cross-check: pointing them at different ` +
        `stacks runs the browser against one and deletes rows in the other.`
    )
  }
  return value
}

export default async function externalPreflight(): Promise<void> {
  const baseURL = required(
    'E2E_BASE_URL (or STACK_FRONTEND_URL)',
    process.env.E2E_BASE_URL ?? process.env.STACK_FRONTEND_URL
  )
  const backendURL = required('BACKEND_URL', process.env.BACKEND_URL)

  // The failure this guards against is a run that goes green against the wrong
  // stack, which is invisible unless the targets are stated up front.
  console.log(`[e2e:external] frontend=${baseURL} backend=${backendURL}`)
}
