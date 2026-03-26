'use client'

import { useState } from 'react'
import { Pencil, Loader2, X, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

const MAX_DESCRIPTION_LENGTH = 5000

interface EntityDescriptionProps {
  description: string | null | undefined
  canEdit: boolean
  onSave: (description: string) => Promise<void>
}

/**
 * Render a description as simple paragraphs.
 * Splits on double newlines for paragraph breaks, preserves single newlines as <br>.
 */
function DescriptionContent({ text }: { text: string }) {
  const paragraphs = text.split(/\n\n+/)

  return (
    <div className="space-y-3">
      {paragraphs.map((paragraph, i) => {
        const lines = paragraph.split('\n')
        return (
          <p key={i} className="text-sm text-muted-foreground leading-relaxed">
            {lines.map((line, j) => (
              <span key={j}>
                {j > 0 && <br />}
                {line}
              </span>
            ))}
          </p>
        )
      })}
    </div>
  )
}

export function EntityDescription({
  description,
  canEdit,
  onSave,
}: EntityDescriptionProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const hasDescription = !!description && description.trim().length > 0

  const handleStartEdit = () => {
    setEditValue(description || '')
    setError(null)
    setIsEditing(true)
  }

  const handleCancel = () => {
    setIsEditing(false)
    setEditValue('')
    setError(null)
  }

  const handleSave = async () => {
    if (editValue.length > MAX_DESCRIPTION_LENGTH) {
      setError(`Description must be ${MAX_DESCRIPTION_LENGTH} characters or fewer`)
      return
    }

    setIsSaving(true)
    setError(null)

    try {
      await onSave(editValue.trim())
      setIsEditing(false)
      setEditValue('')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to save description'
      )
    } finally {
      setIsSaving(false)
    }
  }

  // Edit mode
  if (isEditing) {
    return (
      <div className="space-y-3">
        <Textarea
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          placeholder="Add a description..."
          rows={6}
          maxLength={MAX_DESCRIPTION_LENGTH}
          disabled={isSaving}
          className="resize-y text-sm"
        />
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {editValue.length.toLocaleString()} / {MAX_DESCRIPTION_LENGTH.toLocaleString()}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancel}
              disabled={isSaving}
            >
              <X className="h-4 w-4 mr-1" />
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSave}
              disabled={isSaving}
            >
              {isSaving ? (
                <Loader2 className="h-4 w-4 mr-1 animate-spin" />
              ) : (
                <Check className="h-4 w-4 mr-1" />
              )}
              Save
            </Button>
          </div>
        </div>
        {error && (
          <p className="text-xs text-destructive">{error}</p>
        )}
      </div>
    )
  }

  // Display mode with description
  if (hasDescription) {
    return (
      <div className="group relative">
        <DescriptionContent text={description!} />
        {canEdit && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleStartEdit}
            className="mt-2 h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
          >
            <Pencil className="h-3 w-3 mr-1" />
            Edit description
          </Button>
        )}
      </div>
    )
  }

  // Empty state
  if (canEdit) {
    return (
      <button
        onClick={handleStartEdit}
        className="w-full rounded-lg border border-dashed border-muted-foreground/25 bg-muted/30 p-4 text-left transition-colors hover:bg-muted/50"
      >
        <p className="text-sm text-muted-foreground">
          No description yet.{' '}
          <span className="text-primary hover:underline">Add description</span>
        </p>
      </button>
    )
  }

  // No description, can't edit — show nothing
  return null
}
