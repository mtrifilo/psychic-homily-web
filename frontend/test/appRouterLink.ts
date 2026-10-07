import Module from 'node:module'

/**
 * Lets the App Router Link load under Vitest. vitest.config.mts aliases
 * `next/link` to `next/dist/client/app-dir/link`, the build the app's compiler
 * resolves it to (next/dist/build/create-compiler-aliases.js). That Link's
 * module graph is Node-required CommonJS and requires
 * `react-server-dom-webpack/client`, which Next's compiler aliases to its
 * compiled copy and which is not installed as a package. Vite aliases do not
 * reach a Node `require`, so this file maps that one request in Node's
 * resolver, and defines the webpack module hook the compiled client reads when
 * it loads. Nothing in a unit test resolves a server reference through it.
 */
const FLIGHT_CLIENT = 'react-server-dom-webpack/client'
const COMPILED_FLIGHT_CLIENT = 'next/dist/compiled/react-server-dom-webpack/client'

type Resolver = { _resolveFilename: (request: string, ...rest: unknown[]) => string }
const resolver = Module as unknown as Resolver & { __appRouterLinkPatched?: true }

if (!resolver.__appRouterLinkPatched) {
  const resolve = resolver._resolveFilename
  resolver._resolveFilename = function (request: string, ...rest: unknown[]) {
    return resolve.call(this, request === FLIGHT_CLIENT ? COMPILED_FLIGHT_CLIENT : request, ...rest)
  }
  resolver.__appRouterLinkPatched = true
}

const hooks = globalThis as unknown as Record<string, unknown>
if (hooks.__webpack_require__ === undefined) {
  hooks.__webpack_require__ = () => ({})
}
