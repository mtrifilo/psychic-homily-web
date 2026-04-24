'use client'

import { useState } from 'react'
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
}: CommentFormProps) {
  const [body, setBody] = useState(initialBody)
  const [replyPermission, setReplyPermission] = useState<ReplyPermission>(
    initialReplyPermission
  )

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = body.trim()
    if (!trimmed) return
    onSubmit(trimmed, allowReplyPermission ? replyPermission : undefined)
    if (!initialBody) {
      setBody('')
    }
  }

  const isDisabled = !body.trim() || isPending

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
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
