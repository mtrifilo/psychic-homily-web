'use client'

import { useState, useCallback } from 'react'
import { Lightbulb, X, ArrowRight } from 'lucide-react'
import { useDataGaps } from '../hooks/useDataGaps'
import type { EditableEntityType } from '../types'

/** Map field names to human-readable prompt text. */
const promptTemplates: Record<string, string> = {
  bandcamp: "Know this artist's Bandcamp?",
  spotify: "Know this artist's Spotify?",
  website: 'Know the website?',
  instagram: 'Know the Instagram?',
  city: 'Know where they are based?',
  state: 'Know the state?',
  description: 'Can you write a description?',
  flyer_url: 'Have a flyer for this festival?',
}

/** Fallback prompt when no specific template exists. */
function getPromptText(field: string, entityType: string): string {
  if (promptTemplates[field]) {
    return promptTemplates[field]
  }
  return `Help complete this ${entityType}'s profile`
}

function getDismissalKey(entityType: string, entityId: number): string {
  return `dismissed-prompt-${entityType}-${entityId}`
}

function checkDismissed(entityType: string, entityId: number): boolean {
  try {
    return localStorage.getItem(getDismissalKey(entityType, entityId)) === 'true'
  } catch {
    return false
  }
}

interface ContributionPromptProps {
  entityType: EditableEntityType
  entityId: number
  entitySlug: string
  /** Whether the user is authenticated. If false, renders nothing. */
  isAuthenticated: boolean
  /** Called when user clicks the action button. Opens the edit drawer. */
  onEditClick: () => void
}

export function ContributionPrompt({
  entityType,
  entityId,
  entitySlug,
  isAuthenticated,
  onEditClick,
}: ContributionPromptProps) {
  const [isDismissed, setIsDismissed] = useState(() =>
    checkDismissed(entityType, entityId)
  )

  const { data, isLoading } = useDataGaps(entityType, entitySlug, {
    enabled: isAuthenticated && !isDismissed,
  })

  const handleDismiss = useCallback(() => {
    setIsDismissed(true)
    try {
      localStorage.setItem(getDismissalKey(entityType, entityId), 'true')
    } catch {
      // localStorage unavailable
    }
  }, [entityType, entityId])

  // Don't render anything if:
  // - Not authenticated
  // - Dismissed
  // - Loading
  // - No gaps
  if (!isAuthenticated || isDismissed || isLoading) {
    return null
  }

  const gaps = data?.gaps ?? []
  if (gaps.length === 0) {
    return null
  }

  // Show the highest-priority gap (lowest priority number = first in array, already sorted by backend)
  const topGap = gaps[0]
  const promptText = getPromptText(topGap.field, entityType)

  return (
    <div
      className="flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-sm"
      data-testid="contribution-prompt"
    >
      <Lightbulb className="h-4 w-4 text-primary shrink-0" />
      <span className="text-muted-foreground flex-1">{promptText}</span>
      <button
        onClick={onEditClick}
        className="inline-flex items-center gap-1 text-primary hover:text-primary/80 font-medium whitespace-nowrap transition-colors"
      >
        Add it
        <ArrowRight className="h-3.5 w-3.5" />
      </button>
      <button
        onClick={handleDismiss}
        className="text-muted-foreground/60 hover:text-muted-foreground transition-colors shrink-0"
        aria-label="Dismiss"
        data-testid="dismiss-prompt"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
