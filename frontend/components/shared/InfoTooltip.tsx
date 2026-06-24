'use client'

import { Info } from 'lucide-react'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export interface InfoTooltipProps {
  /** Explainer copy shown inside the tooltip content. */
  copy: string
  /** Accessible name for the trigger button. */
  label: string
  /** Tooltip placement relative to the glyph (default 'top'). */
  side?: 'top' | 'bottom' | 'left' | 'right'
  /** testid passthrough so call sites keep their existing test hooks. */
  testId?: string
}

/**
 * Shared ⓘ-glyph explainer (PSY-969). A small, keyboard- and hover-accessible
 * info icon that reveals a short copy string in a Radix tooltip.
 *
 * Extracted from two byte-for-byte-identical local implementations
 * (TagFacetPanel's `TransitiveTagTooltip` and AddItemsPicker's
 * `AiTabInfoTooltip`). Owns its own `TooltipProvider` so it drops in anywhere
 * without a surrounding provider — matching the prior call-site behavior.
 *
 * The glyph size is hardcoded at `h-3.5 w-3.5` (no prop until a third call
 * site needs a different size). The button styling is the exact chrome the
 * two call sites shipped, so rendered output is pixel-identical.
 *
 * Import via the subpath (`@/components/shared/InfoTooltip`), NOT the feature
 * barrel — a barrel import re-bloats the Turbopack global shared chunk on
 * browse-route-reachable call sites.
 *
 * Usage:
 *   <InfoTooltip copy={text} label="How tag filtering works" testId="…" />
 */
export function InfoTooltip({
  copy,
  label,
  side = 'top',
  testId,
}: InfoTooltipProps) {
  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={label}
            className="inline-flex items-center rounded-full p-0.5 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid={testId}
          >
            <Info className="h-3.5 w-3.5" aria-hidden />
          </button>
        </TooltipTrigger>
        <TooltipContent side={side} className="max-w-xs text-xs">
          {copy}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
