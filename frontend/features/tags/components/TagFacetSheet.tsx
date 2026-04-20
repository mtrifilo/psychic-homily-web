'use client'

import { useState } from 'react'
import { Filter } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { TagFacetPanel, type TagFacetPanelProps } from './TagFacetPanel'

export interface TagFacetSheetProps
  extends Omit<TagFacetPanelProps, 'heading' | 'hideHeading'> {
  /** Button label (defaults to "Tags"). */
  triggerLabel?: string
  /** Drawer heading. */
  title?: string
  /** Optional sub-copy under the title. */
  description?: string
}

/**
 * Mobile-first wrapper around `TagFacetPanel`. Browse pages render the
 * panel as a sidebar on desktop and this sheet as a drawer trigger on
 * mobile (PSY-309 spec). Selection state is always lifted — the sheet
 * just delegates rendering to `TagFacetPanel`.
 */
export function TagFacetSheet({
  triggerLabel = 'Tags',
  title = 'Filter by tags',
  description,
  selectedSlugs,
  ...panelProps
}: TagFacetSheetProps) {
  const [open, setOpen] = useState(false)
  const count = selectedSlugs.length
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="lg:hidden"
          data-testid="tag-facet-sheet-trigger"
        >
          <Filter className="mr-1.5 h-3.5 w-3.5" />
          {triggerLabel}
          {count > 0 && (
            <span className="ml-1.5 rounded-full bg-primary px-1.5 py-0.5 text-[10px] font-semibold text-primary-foreground">
              {count}
            </span>
          )}
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-sm">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>
        <div className="px-4 pb-6">
          <TagFacetPanel
            {...panelProps}
            selectedSlugs={selectedSlugs}
            hideHeading
          />
        </div>
      </SheetContent>
    </Sheet>
  )
}
