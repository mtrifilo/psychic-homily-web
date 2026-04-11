'use client'

import { useState } from 'react'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { Loader2 } from 'lucide-react'

interface CommentFormProps {
  onSubmit: (body: string) => void
  initialBody?: string
  placeholder?: string
  submitLabel?: string
  onCancel?: () => void
  isPending?: boolean
}

export function CommentForm({
  onSubmit,
  initialBody = '',
  placeholder = 'Share your thoughts...',
  submitLabel = 'Post',
  onCancel,
  isPending = false,
}: CommentFormProps) {
  const [body, setBody] = useState(initialBody)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = body.trim()
    if (!trimmed) return
    onSubmit(trimmed)
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
      <div className="flex items-center gap-2">
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
      </div>
    </form>
  )
}
