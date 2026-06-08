import { type Page } from '@playwright/test'
import { test, expect } from '../fixtures'
import {
  resetTestFixtures,
  lookupWorkerUserId,
} from '../fixtures/test-fixtures-reset'
import { USER_COUNT, userAuthFileForWorker } from '../global-setup'

// Auth-gated buttons (FollowButton, AttendanceButton) redirect to /auth
// when useAuthContext returns isAuthenticated=false at click time. The
// buttons render regardless of auth state (only tooltip copy differs),
// so visibility checks don't gate against the SSR-pre-hydration → client
// propagation race window. The account "User menu" avatar button (the top
// bar's UserMenu) is rendered only when isAuthenticated=true, so waiting on
// it is a reliable signal that auth has propagated before we click an
// auth-gated button. (PSY-1013 retired the sidebar — which previously
// carried this auth-only "Library" link — in favour of the top-bar account
// menu; Library now lives inside that dropdown.)
async function waitForAuthHydrated(page: Page) {
  await expect(
    page.getByRole('button', { name: 'User menu' })
  ).toBeVisible({ timeout: 10_000 })
}

// PSY-457: backfill E2E coverage for follow + going/interested flows.
// PSY-430: pin to reserved artist + show seeded by setup-db.sh so parallel
// mutating tests in other files don't race on the same .first() row.
const RESERVED_ARTIST_SLUG = 'e2e-follow-test'
const RESERVED_ARTIST_NAME = 'E2E [follow-test]'
const RESERVED_ARTIST_URL = `/artists/${RESERVED_ARTIST_SLUG}`

const RESERVED_SHOW_SLUG = 'e2e-attendance-test'
const RESERVED_SHOW_TITLE = 'E2E [attendance-test]'
const RESERVED_SHOW_URL = `/shows/${RESERVED_SHOW_SLUG}`

test.describe('Follow and attendance', () => {
  // Tests share DB state with the same per-worker user, so they must not
  // run in parallel within this file.
  test.describe.configure({ mode: 'serial' })

  // PSY-470: the in-test cleanup at the happy-path tail only runs when the
  // test passes. When a mid-test failure skips it (see PSY-465), the shared
  // reserved artist/show rows leak follower/attendance count into the next
  // repeat and cause cascading `followers_count` mismatches across workers.
  // workerCleanup (PSY-432) only fires on worker teardown, not between
  // tests. Scope the reset to just `user_bookmarks` — narrower than the
  // teardown reset so it stays cheap when run per test.
  let workerUserId: number | null = null

  test.beforeAll(async ({}, testInfo) => {
    const seededIndex = testInfo.workerIndex % USER_COUNT
    workerUserId = await lookupWorkerUserId(userAuthFileForWorker(seededIndex))
  })

  test.afterEach(async () => {
    if (workerUserId !== null) {
      await resetTestFixtures(workerUserId, ['user_bookmarks'])
    }
  })

  test('follow an artist round-trip surfaces in Library', { tag: '@smoke' }, async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto(RESERVED_ARTIST_URL)

    // Wait for artist detail to load (H1 renders the artist name)
    await expect(
      authenticatedPage.getByRole('heading', {
        level: 1,
        name: RESERVED_ARTIST_NAME,
      })
    ).toBeVisible({ timeout: 10_000 })

    await waitForAuthHydrated(authenticatedPage)

    // FollowButton on the artist detail page renders as a BracketLink
    // (variant="bracket" since PSY-641). The bracket variant uses the
    // label string for the accessible name and deliberately omits the
    // follower count for dense linkbox chrome — initial state "Follow",
    // post-click "Following" (with aria-pressed="true"), no count span.
    const followButton = authenticatedPage.getByRole('button', {
      name: 'Follow',
      exact: true,
    })
    await expect(followButton).toBeVisible({ timeout: 5_000 })

    // Click Follow — wait for POST /artists/{id}/follow to complete so DB
    // state is settled before we navigate to Library (optimistic UI flips
    // the text before the request completes).
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          /\/artists\/\d+\/follow$/.test(resp.url()) &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      followButton.click(),
    ])

    // Button flips to "Following". Bracket variant has no hover-to-Unfollow
    // state, so we just assert the followed-state label — the round-trip
    // assertion is "is the artist now in the user's library?" below.
    await expect(
      authenticatedPage.getByRole('button', {
        name: 'Following',
        exact: true,
      })
    ).toBeVisible({ timeout: 5_000 })

    // Navigate to Library Artists tab and verify the followed artist
    // surfaces there via FollowingEntityCard's link.
    await authenticatedPage.goto('/library?tab=artists')
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      authenticatedPage.getByRole('link', { name: RESERVED_ARTIST_NAME })
    ).toBeVisible({ timeout: 5_000 })

    // Cleanup: navigate back and unfollow so the test is idempotent.
    await authenticatedPage.goto(RESERVED_ARTIST_URL)
    await expect(
      authenticatedPage.getByRole('heading', {
        level: 1,
        name: RESERVED_ARTIST_NAME,
      })
    ).toBeVisible({ timeout: 10_000 })

    await waitForAuthHydrated(authenticatedPage)

    // Clicking the [Following] bracket link while isFollowing=true triggers
    // unfollow. Bracket variant has no hover-to-Unfollow state, so the
    // accessible name stays "Following" until the click resolves.
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          /\/artists\/\d+\/follow$/.test(resp.url()) &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: 'Following', exact: true })
        .click(),
    ])

    // Button should revert to "Follow" (count=0, no count span).
    await expect(
      authenticatedPage.getByRole('button', { name: 'Follow', exact: true })
    ).toBeVisible({ timeout: 5_000 })
  })

  test('mark show as going round-trip surfaces in Library', { tag: '@smoke' }, async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto(RESERVED_SHOW_URL)

    // Breadcrumb shows the show title; the H1 is the headlining artist name,
    // so we verify the right show loaded via the breadcrumb.
    await expect(
      authenticatedPage
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    await waitForAuthHydrated(authenticatedPage)

    // Going button initial state: no count suffix on a fresh reserved show
    // (accessible name = "Going").
    const goingButton = authenticatedPage.getByRole('button', {
      name: 'Going',
      exact: true,
    })
    await expect(goingButton).toBeVisible({ timeout: 5_000 })

    // Click Going — wait for POST /shows/{id}/attendance to complete so DB
    // state is settled before we navigate to Library.
    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          /\/shows\/\d+\/attendance$/.test(resp.url()) &&
          resp.request().method() === 'POST',
        { timeout: 10_000 }
      ),
      goingButton.click(),
    ])

    // Button's accessible name now includes the going count ("Going 1").
    await expect(
      authenticatedPage.getByRole('button', { name: /^Going\s*1$/ })
    ).toBeVisible({ timeout: 5_000 })

    // Navigate to Library Shows tab and verify the attending show surfaces
    // under the "Going / Interested" section via AttendingShowCard.
    await authenticatedPage.goto('/library?tab=shows')
    await expect(
      authenticatedPage.getByRole('heading', { name: /^library$/i })
    ).toBeVisible({ timeout: 10_000 })
    await expect(
      authenticatedPage.getByRole('link', { name: RESERVED_SHOW_TITLE })
    ).toBeVisible({ timeout: 5_000 })

    // Cleanup: navigate back and unmark attendance so the test is
    // idempotent. Clicking the same status (Going) while already going
    // removes attendance via DELETE.
    await authenticatedPage.goto(RESERVED_SHOW_URL)
    await expect(
      authenticatedPage
        .getByRole('navigation', { name: 'Breadcrumb' })
        .getByText(RESERVED_SHOW_TITLE)
    ).toBeVisible({ timeout: 10_000 })

    await waitForAuthHydrated(authenticatedPage)

    await Promise.all([
      authenticatedPage.waitForResponse(
        (resp) =>
          /\/shows\/\d+\/attendance$/.test(resp.url()) &&
          resp.request().method() === 'DELETE',
        { timeout: 10_000 }
      ),
      authenticatedPage
        .getByRole('button', { name: /^Going\s*1$/ })
        .click(),
    ])

    // Button should revert to "Going" with no count.
    await expect(
      authenticatedPage.getByRole('button', { name: 'Going', exact: true })
    ).toBeVisible({ timeout: 5_000 })
  })
})
