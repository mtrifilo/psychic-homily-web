/**
 * A sign-up control's claim on /auth while a registration is in flight or has
 * just landed. While any claim is held the page does not run its
 * already-authenticated redirect.
 *
 * Two phases rather than one, because the viewer can become authenticated
 * before the control that created the account has handled its own success:
 * `useRegister` awaits a full `/auth/profile` refetch inside its own
 * `onSuccess`, and any profile refetch can report a session the moment its
 * cookie exists. A one-phase "set the email when it succeeds" handoff would let
 * the redirect fire in that window and navigate away from a surface that had
 * not been claimed yet. `pending` is set before the request that creates the
 * session goes out, which closes that window entirely.
 *
 * `method` is the factor the account was created with, and it decides what the
 * page shows in place of the form. After a password: the check-your-inbox
 * interstitial. After a passkey: the backup sign-in prompt first (`step:
 * 'backup-prompt'`), and once the prompt closes, by any route, the same
 * interstitial (`step: 'check-inbox'`). A completed claim is held until the
 * page unmounts, so the interstitial's own links are the way on.
 */
export type SignupHandoff =
  | { status: 'pending'; email: string }
  | { status: 'complete'; email: string; method: 'password' }
  | {
      status: 'complete'
      email: string
      method: 'passkey'
      step: 'backup-prompt' | 'check-inbox'
    }
