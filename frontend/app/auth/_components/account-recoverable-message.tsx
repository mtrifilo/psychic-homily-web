import Link from 'next/link'

export const ACCOUNT_RECOVERABLE_MESSAGE =
  'This account was deleted recently and can still be recovered.'
export const ACCOUNT_RECOVERABLE_LINK_TEXT = 'Recover your account'

/**
 * What the auth page shows for ACCOUNT_RECOVERABLE, wherever a sign-in is
 * refused with it: the password form's alert and the OAuth refusal banner.
 * Fixed copy: the server's message for the code is not rendered here.
 */
export function AccountRecoverableMessage() {
  return (
    <p>
      {ACCOUNT_RECOVERABLE_MESSAGE}{' '}
      <Link href="/auth/recover" className="font-medium underline underline-offset-4">
        {ACCOUNT_RECOVERABLE_LINK_TEXT}
      </Link>
    </p>
  )
}
