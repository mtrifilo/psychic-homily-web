import { test, expect } from '../fixtures'

// PSY-456: golden-path E2E coverage for the general comments surface
// (create / vote / reply). Field notes are out of scope (separate ticket).
//
// Each test uses its own reserved venue (see setup-db.sh) so the 60s
// per-entity comment cooldown cannot collide across create/vote/reply
// within this spec. Worker users are seeded as `contributor` tier in
// setup-db.sh so new comments publish as `visible` immediately; without
// that, `new_user` tier would route comments to `pending_review` and
// the rendered UI would not match the assertions below.
//
// The vote test asserts the per-user button-state flip (not score
// math) so it is race-free under parallel workers voting on the same
// admin-seeded target. Wilson-score math is Go-tested (see
// backend/internal/services/engagement/comment_vote_service_test.go).

const CREATE_VENUE_SLUG = 'e2e-comment-create'
const CREATE_VENUE_NAME = 'E2E [comment-create]'

const VOTE_VENUE_SLUG = 'e2e-comment-vote'
const VOTE_VENUE_NAME = 'E2E [comment-vote]'
// Body of the admin-seeded vote-target comment (see setup-db.sh).
const VOTE_TARGET_BODY = 'E2E vote-target seed comment'

const REPLY_VENUE_SLUG = 'e2e-comment-reply'
const REPLY_VENUE_NAME = 'E2E [comment-reply]'
// Body of the admin-seeded parent comment (see setup-db.sh).
const REPLY_PARENT_BODY = 'E2E reply-parent seed comment'

test.describe('Comments (general)', () => {
  test(
    'authenticated user creates a comment on an entity',
    { tag: '@smoke' },
    async ({ authenticatedPage }) => {
      await authenticatedPage.goto(`/venues/${CREATE_VENUE_SLUG}`)

      // Confirm we loaded the right venue before we mutate.
      await expect(
        authenticatedPage.getByRole('heading', {
          level: 1,
          name: CREATE_VENUE_NAME,
        })
      ).toBeVisible({ timeout: 10_000 })

      // Wait for the thread region to mount (hydrates client-side after
      // the initial venue fetch).
      const thread = authenticatedPage.getByTestId('comment-thread')
      await expect(thread).toBeVisible({ timeout: 10_000 })

      const uniqueBody = `E2E comment create ${Date.now()}`
      await thread
        .getByTestId('comment-textarea')
        .fill(uniqueBody)

      // PSY-430: pair the submit click with waitForResponse so the POST
      // settles before we assert on the rendered list (contributor tier
      // => visibility=visible, so the comment should appear on refetch).
      // PSY-462: bumped timeout to 30s to absorb CI cold-start cost on the
      // first backend request of the spec.
      const [createResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            resp.url().includes('/entities/venue/') &&
            resp.url().endsWith('/comments') &&
            resp.request().method() === 'POST',
          { timeout: 30_000 }
        ),
        thread.getByTestId('comment-submit').click(),
      ])
      expect(createResp.status()).toBeLessThan(400)
      const createBody = (await createResp.json()) as { id: number }
      const createdId = createBody.id

      // The new comment renders via the list refetch triggered by the
      // mutation's onSuccess invalidation.
      await expect(thread.getByText(uniqueBody)).toBeVisible({ timeout: 5_000 })

      // Cleanup: delete the comment we just created so the test is
      // idempotent across re-runs on the same DB snapshot.
      const [deleteResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            resp.url().includes(`/comments/${createdId}`) &&
            resp.request().method() === 'DELETE',
          { timeout: 10_000 }
        ),
        authenticatedPage.request.delete(`/api/comments/${createdId}`),
      ])
      expect(deleteResp.status()).toBeLessThan(400)
    }
  )

  test(
    'authenticated user upvotes a comment',
    async ({ authenticatedPage }) => {
      await authenticatedPage.goto(`/venues/${VOTE_VENUE_SLUG}`)

      await expect(
        authenticatedPage.getByRole('heading', {
          level: 1,
          name: VOTE_VENUE_NAME,
        })
      ).toBeVisible({ timeout: 10_000 })

      const thread = authenticatedPage.getByTestId('comment-thread')
      await expect(thread).toBeVisible({ timeout: 10_000 })

      // Locate the admin-seeded target comment by its body text and climb
      // to the enclosing comment card so we scope all assertions/clicks
      // to just that comment.
      const targetCard = thread
        .locator('[data-testid="comment-card"]', {
          hasText: VOTE_TARGET_BODY,
        })
        .first()
      await expect(targetCard).toBeVisible({ timeout: 5_000 })

      const upvoteButton = targetCard.getByTestId('upvote-button')

      // Upvote. Wait for the server response so the optimistic state is
      // confirmed before we assert on the button state flip.
      const [voteResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            /\/comments\/\d+\/vote$/.test(resp.url()) &&
            resp.request().method() === 'POST',
          { timeout: 10_000 }
        ),
        upvoteButton.click(),
      ])
      expect(voteResp.status()).toBeLessThan(400)

      // Vote state flipped: the upvote button picks up the active color
      // class (`text-primary`) when `user_vote === 1`. This is a per-user
      // signal that is race-free under parallel workers hitting the same
      // seed comment. Raw score + Wilson-score math are Go-tested (see
      // comment_vote_service_test.go) so we don't assert on those here.
      await expect(upvoteButton).toHaveClass(/text-primary/, { timeout: 5_000 })

      // Cleanup: remove our vote so re-runs start from a clean per-user
      // state (other workers' votes persist; that's fine).
      const [unvoteResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            /\/comments\/\d+\/vote$/.test(resp.url()) &&
            resp.request().method() === 'DELETE',
          { timeout: 10_000 }
        ),
        upvoteButton.click(),
      ])
      expect(unvoteResp.status()).toBeLessThan(400)
    }
  )

  test(
    'authenticated user replies to a comment (nested, depth <= 2)',
    { tag: '@smoke' },
    async ({ authenticatedPage }) => {
      await authenticatedPage.goto(`/venues/${REPLY_VENUE_SLUG}`)

      await expect(
        authenticatedPage.getByRole('heading', {
          level: 1,
          name: REPLY_VENUE_NAME,
        })
      ).toBeVisible({ timeout: 10_000 })

      const thread = authenticatedPage.getByTestId('comment-thread')
      await expect(thread).toBeVisible({ timeout: 10_000 })

      // Locate the admin-seeded parent comment.
      const parentCard = thread
        .locator('[data-testid="comment-card"]', {
          hasText: REPLY_PARENT_BODY,
        })
        .first()
      await expect(parentCard).toBeVisible({ timeout: 5_000 })

      // Open the reply form. Before the form renders, the only "Reply"
      // button inside the parent card is the toggle; after it renders,
      // we switch to the form's submit testid for disambiguation.
      await parentCard
        .getByRole('button', { name: 'Reply', exact: true })
        .click()

      const uniqueReply = `E2E reply ${Date.now()}`
      await parentCard.getByTestId('comment-textarea').fill(uniqueReply)

      // Submit the reply and wait for the server POST to settle.
      const [replyResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            /\/comments\/\d+\/replies$/.test(resp.url()) &&
            resp.request().method() === 'POST',
          { timeout: 10_000 }
        ),
        parentCard.getByTestId('comment-submit').click(),
      ])
      expect(replyResp.status()).toBeLessThan(400)
      const replyBody = (await replyResp.json()) as {
        id: number
        depth: number
        parent_id: number | null
      }
      const replyId = replyBody.id

      // Backend invariants: depth > 0 and parent_id set. MaxCommentDepth
      // is 2 in backend/internal/models/comment.go (0/1/2 = 3 levels);
      // this first-level reply should land at depth 1.
      expect(replyBody.parent_id).not.toBeNull()
      expect(replyBody.depth).toBeGreaterThan(0)
      expect(replyBody.depth).toBeLessThanOrEqual(2)

      // The reply renders nested under the parent. PSY-462: bumped timeout
      // to 15s to absorb CI cost of invalidation -> refetch -> nested render.
      await expect(parentCard.getByText(uniqueReply)).toBeVisible({
        timeout: 15_000,
      })

      // Cleanup: delete the reply so re-runs are idempotent.
      const [deleteResp] = await Promise.all([
        authenticatedPage.waitForResponse(
          (resp) =>
            resp.url().includes(`/comments/${replyId}`) &&
            resp.request().method() === 'DELETE',
          { timeout: 10_000 }
        ),
        authenticatedPage.request.delete(`/api/comments/${replyId}`),
      ])
      expect(deleteResp.status()).toBeLessThan(400)
    }
  )
})
