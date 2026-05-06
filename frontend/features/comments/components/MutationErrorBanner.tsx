'use client'

/**
 * Inline error banner for comment / field-note mutations (PSY-608).
 *
 * Shared visual shape for sticky-until-retry banners (delete, reply
 * permission) and auto-dismiss banners (vote / unvote). Keep the
 * destructive styling consistent across all surfaces inside the comments
 * feature module — the `errorMessage` slots inside `CommentForm` /
 * `FieldNoteForm` use a slightly larger pad (`p-3`) since they sit above a
 * form field; this banner uses `px-3 py-2` because it's anchored next to
 * an action row.
 *
 * Internal to the comments feature — not re-exported from the public
 * `components/index.ts`.
 */
interface MutationErrorBannerProps {
  message: string
  testId: string
  /** Margin-top override; defaults to `mt-2`. FieldNoteCard uses `mt-3`. */
  marginTop?: 'mt-2' | 'mt-3'
}

export function MutationErrorBanner({
  message,
  testId,
  marginTop = 'mt-2',
}: MutationErrorBannerProps) {
  return (
    <div
      className={`${marginTop} rounded-md border border-red-800 bg-red-950/50 px-3 py-2`}
      role="alert"
      data-testid={testId}
    >
      <p className="text-sm text-red-400">{message}</p>
    </div>
  )
}
