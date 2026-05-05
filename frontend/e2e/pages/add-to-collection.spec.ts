import { test, expect } from '../fixtures'

// PSY-455: E2E coverage for the add-to-collection golden path.
// Phase 2a shipped the collections UX overhaul without E2E coverage; this
// smoke exercises the PMF-critical flow from an entity detail page.
//
// PSY-430 reserved-row pattern: pin to a dedicated reserved show so parallel
// mutating specs in other files don't race on the same .first() row.
//
// PSY-431 per-worker users: each worker-user has its own pre-seeded
// "E2E Worker Collection" (see setup-db.sh) so the test doesn't have to
// create one and doesn't race on shared collection state.
const RESERVED_SHOW_SLUG = 'e2e-add-to-collection-test'
const RESERVED_SHOW_TITLE = 'E2E [add-to-collection-test]'
const RESERVED_SHOW_URL = `/shows/${RESERVED_SHOW_SLUG}`
const RESERVED_COLLECTION_TITLE = 'E2E Worker Collection'

test.describe('Add to Collection', () => {
  test(
    'adds a show to a worker-owned collection from the detail page',
    { tag: '@smoke' },
    async ({ authenticatedPage }) => {
      // 1. Navigate to the reserved show detail page.
      await authenticatedPage.goto(RESERVED_SHOW_URL)
      // Breadcrumb shows the show title; the H1 is the headlining artist name,
      // so we verify the right show loaded via the breadcrumb.
      await expect(
        authenticatedPage
          .getByRole('navigation', { name: 'Breadcrumb' })
          .getByText(RESERVED_SHOW_TITLE)
      ).toBeVisible({ timeout: 10_000 })

      // 2. Open the Add to Collection popover.
      // The trigger button has aria-label="Add to Collection" and visible
      // text "Collect" (AddToCollectionButton.tsx).
      const collectButton = authenticatedPage.getByRole('button', {
        name: 'Add to Collection',
      })
      await expect(collectButton).toBeVisible({ timeout: 5_000 })
      await collectButton.click()

      // 3. Pick the pre-seeded "E2E Worker Collection" from the picker.
      // PSY-359 rebuilt the picker into a multi-select: each collection is a
      // checkbox (accessible name = collection title) and submission happens
      // through a single bottom "Add to N collection(s)" button.
      const collectionCheckbox = authenticatedPage.getByRole('checkbox', {
        name: RESERVED_COLLECTION_TITLE,
      })
      await expect(collectionCheckbox).toBeVisible({ timeout: 5_000 })
      await collectionCheckbox.click()

      const submitButton = authenticatedPage.getByRole('button', {
        name: /Add to 1 collection/,
      })

      // PSY-430: waitForResponse wraps the mutation so we don't race on the
      // optimistic UI state — the popover updates before the request completes.
      const [addResponse] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            resp.url().includes('/collections/') &&
            resp.url().includes('/items') &&
            resp.request().method() === 'POST',
          { timeout: 10_000 }
        ),
        submitButton.click(),
      ])
      expect(addResponse.status()).toBeLessThan(400)

      // Extract the collection slug from the mutation URL so we can navigate
      // to the exact collection owned by this worker-user.
      // URL shape: .../collections/<slug>/items
      const slugMatch = addResponse.url().match(/\/collections\/([^/]+)\/items/)
      expect(slugMatch).not.toBeNull()
      const collectionSlug = slugMatch![1]

      // 4. Confirm the popover reflects success: the checkbox stays checked
      // (PSY-359 keeps the row in `savedIds` after a successful add).
      await expect(collectionCheckbox).toBeChecked()

      // 5. Navigate to the collection detail page.
      await authenticatedPage.goto(`/collections/${collectionSlug}`)
      await expect(
        authenticatedPage.getByRole('heading', {
          name: RESERVED_COLLECTION_TITLE,
        })
      ).toBeVisible({ timeout: 10_000 })

      // PSY-360 made grid view the default for collection items. The grid
      // card (CollectionItemCard) doesn't expose a per-item Remove control;
      // only the list-view row does. Switch to list view so the cleanup
      // selectors below — which target the list-view row layout
      // (div.rounded-lg wrapper + title="Remove from collection") — keep
      // working. The view toggle renders unconditionally in the items
      // header, so awaiting the click is safe.
      await authenticatedPage.getByTestId('view-mode-list').click()

      // 6. Verify the show appears in the collection's items list.
      // Each item links to the entity via entity_name as link text.
      const itemLink = authenticatedPage.getByRole('link', {
        name: RESERVED_SHOW_TITLE,
      })
      await expect(itemLink).toBeVisible({ timeout: 5_000 })
      await expect(itemLink).toHaveAttribute(
        'href',
        `/shows/${RESERVED_SHOW_SLUG}`
      )

      // 7. Cleanup — remove the item so the test is idempotent across reruns.
      // The remove flow is two-step: click the X (title="Remove from collection"),
      // then confirm by clicking the "Remove" button that replaces it. Scope to
      // the item row so the selectors stay specific even if other items land
      // in the collection in the future.
      const itemRow = authenticatedPage
        .locator('div.rounded-lg')
        .filter({ has: itemLink })
        .first()

      await itemRow.getByTitle('Remove from collection').click()

      await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            resp.url().includes(`/collections/${collectionSlug}/items/`) &&
            resp.request().method() === 'DELETE',
          { timeout: 10_000 }
        ),
        itemRow.getByRole('button', { name: 'Remove', exact: true }).click(),
      ])

      // The item should be gone after the DELETE completes.
      await expect(itemLink).not.toBeVisible({ timeout: 5_000 })
    }
  )
})
