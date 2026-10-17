/**
 * Whether the catch-all `/api/*` proxy (app/api/[...path]/route.ts) forwards
 * to the backend in this process.
 *
 * Only development serves it: `next dev`, which the Playwright harness and the
 * dispatch stacks run, and a built app started with `ENABLE_DEV_API_PROXY=1`.
 * Deployed builds call the backend origin directly, so anywhere else the proxy
 * would only be a second public path to the backend, and the route answers 404.
 *
 * `next build` inlines `NODE_ENV`, so a production bundle decides the first
 * operand at build time. `ENABLE_DEV_API_PROXY` is not a `NEXT_PUBLIC_`
 * variable and is read from the server's environment at request time.
 */
export function isDevApiProxyEnabled(): boolean {
  return (
    process.env.NODE_ENV === 'development' ||
    process.env.ENABLE_DEV_API_PROXY === '1'
  )
}
