import { test, expect } from '../fixtures'

// PSY-551: regression test for "private collection detail page 404s for its
// own owner". The page route ran an SSR fetch against the backend without
// forwarding the viewer's auth cookie, so the backend returned 404 (correct
// for anon viewers of private collections) and the page called notFound()
// even when rendered for the creator. Fixed by forwarding `auth_token` from
// `next/headers` into the SSR fetch (see app/collections/[slug]/page.tsx).
//
// PSY-432: worker teardown auto-resets the `collections` table for the
// worker user, so the collection created here doesn't pollute later runs.
const PRIVATE_TITLE = `PSY-551 Private ${Date.now()}`

test.describe('Private collection detail (owner access)', () => {
  test(
    'create-private redirects to detail page and renders (not 404)',
    async ({ authenticatedPage }) => {
      // 1. Open the collections browse page and start the create flow.
      await authenticatedPage.goto('/collections')
      await authenticatedPage
        .getByRole('button', { name: 'Create Collection' })
        .first()
        .click()

      // 2. Fill the title and uncheck Public to make it private.
      await authenticatedPage
        .getByLabel('Title', { exact: true })
        .fill(PRIVATE_TITLE)
      const publicCheckbox = authenticatedPage.getByRole('checkbox', {
        name: 'Public',
      })
      // The form defaults Public=on (CollectionList.tsx line 432). Toggle off.
      await expect(publicCheckbox).toBeChecked()
      await publicCheckbox.uncheck()

      // 3. Submit, wait for the create POST, then for the auto-redirect to
      // /collections/<slug>. The redirect is what previously rendered 404.
      const [createResponse] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            /\/collections\/?$/.test(new URL(resp.url()).pathname) &&
            resp.request().method() === 'POST',
          { timeout: 10_000 }
        ),
        authenticatedPage
          .getByRole('button', { name: 'Create', exact: true })
          .click(),
      ])
      expect(createResponse.status()).toBeLessThan(400)
      const created = (await createResponse.json()) as { slug?: string }
      expect(created.slug).toBeTruthy()
      const slug = created.slug as string

      await authenticatedPage.waitForURL(`/collections/${slug}`)

      // 4. Detail page renders the private collection's title (not 404).
      // not-found.tsx renders "Page not found" — assert it's NOT visible to
      // catch regressions where the page falls through to notFound().
      await expect(
        authenticatedPage.getByRole('heading', { name: PRIVATE_TITLE })
      ).toBeVisible({ timeout: 10_000 })
      await expect(
        authenticatedPage.getByText('Page not found', { exact: false })
      ).toHaveCount(0)

      // 5. Hard reload — the SSR fetch (which is what regressed) runs again
      // here, so this is the strongest signal that auth forwarding works.
      await authenticatedPage.reload()
      await expect(
        authenticatedPage.getByRole('heading', { name: PRIVATE_TITLE })
      ).toBeVisible({ timeout: 10_000 })

      // 6. Navigating from the "Yours" tab also runs the SSR detail fetch.
      await authenticatedPage.goto('/collections')
      await authenticatedPage
        .getByRole('tab', { name: 'Yours' })
        .click()
      await authenticatedPage
        .getByRole('link', { name: PRIVATE_TITLE })
        .first()
        .click()
      await authenticatedPage.waitForURL(`/collections/${slug}`)
      await expect(
        authenticatedPage.getByRole('heading', { name: PRIVATE_TITLE })
      ).toBeVisible({ timeout: 10_000 })
    }
  )
})
