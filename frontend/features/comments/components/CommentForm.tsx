'use client'

import { useEffect, useState } from 'react'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { Loader2 } from 'lucide-react'
import { ReplyPermissionSelect } from './ReplyPermissionSelect'
import type { ReplyPermission } from '../types'

interface CommentFormProps {
  onSubmit: (body: string, replyPermission?: ReplyPermission) => void
  initialBody?: string
  placeholder?: string
  submitLabel?: string
  onCancel?: () => void
  isPending?: boolean
  /** PSY-296: when true, shows the reply-permission selector. */
  allowReplyPermission?: boolean
  /** Initial reply-permission value (defaults to user pref or 'anyone'). */
  initialReplyPermission?: ReplyPermission
  /**
   * PSY-589: optional inline error banner. When set, renders a
   * destructive-styled message above the textarea. The form does NOT
   * auto-clear while an error is present so the user can retry without
   * retyping their draft.
   */
  errorMessage?: string | null
  /**
   * PSY-589: bumping this number signals "submission succeeded — clear
   * the textarea." Replaces the old optimistic clear-on-submit behavior
   * which discarded drafts when the request later 4xx'd. Edit-mode
   * callers (which pass `initialBody`) leave this undefined and tear
   * the form down via `onCancel` instead.
   */
  resetSignal?: number
}

export function CommentForm({
  onSubmit,
  initialBody = '',
  placeholder = 'Share your thoughts...',
  submitLabel = 'Post',
  onCancel,
  isPending = false,
  allowReplyPermission = false,
  initialReplyPermission = 'anyone',
  errorMessage,
  resetSignal,
}: CommentFormProps) {
  const [body, setBody] = useState(initialBody)
  const [replyPermission, setReplyPermission] = useState<ReplyPermission>(
    initialReplyPermission
  )

  // PSY-589: parent bumps `resetSignal` from mutation onSuccess. Edit-mode
  // callers don't pass it, so the body is preserved for the edit lifetime.
  useEffect(() => {
    if (resetSignal === undefined) return
    setBody('')
    setReplyPermission(initialReplyPermission)
    // Only fire on resetSignal changes; initialReplyPermission is read
    // lazily on success and shouldn't re-trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetSignal])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = body.trim()
    if (!trimmed) return
    onSubmit(trimmed, allowReplyPermission ? replyPermission : undefined)
  }

  const isDisabled = !body.trim() || isPending

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      {errorMessage && (
        <div
          className="rounded-md border border-red-800 bg-red-950/50 p-3"
          role="alert"
          data-testid="comment-form-error"
        >
          <p className="text-sm text-red-400">{errorMessage}</p>
        </div>
      )}
      <Textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder={placeholder}
        rows={3}
        disabled={isPending}
        data-testid="comment-textarea"
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={isDisabled}
          data-testid="comment-submit"
        >
          {isPending && <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />}
          {submitLabel}
        </Button>
        {onCancel && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            disabled={isPending}
          >
            Cancel
          </Button>
        )}
        {allowReplyPermission && (
          <label className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
            <span className="hidden sm:inline">Who can reply:</span>
            <ReplyPermissionSelect
              value={replyPermission}
              onChange={setReplyPermission}
              disabled={isPending}
              ariaLabel="Who can reply to this comment"
            />
          </label>
        )}
      </div>
    </form>
  )
}
