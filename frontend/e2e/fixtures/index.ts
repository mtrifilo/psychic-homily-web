/**
 * Combined E2E fixtures: error detection + authenticated pages.
 *
 * Usage:
 *   import { test, expect } from '../fixtures'
 *
 *   test('my test', async ({ page, errors }) => { ... })
 *   test('auth test', async ({ authenticatedPage, errors }) => { ... })
 *   test('admin test', async ({ adminPage, errors }) => { ... })
 */
export { test, expect } from './auth'
