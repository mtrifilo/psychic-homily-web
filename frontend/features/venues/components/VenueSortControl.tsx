'use client'

import { useState } from 'react'
import { Check } from 'lucide-react'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { cn } from '@/lib/utils'
import {
  VENUE_SORTS,
  VENUE_SORT_LABELS,
  type VenueSort,
} from '../venuesListNavigation'

export interface VenueSortControlProps {
  /** The order in force. */
  sort: VenueSort
  /** Applies a new order. The consumer owns the URL write and the page reset. */
  onSortChange: (sort: VenueSort) => void
  className?: string
}

/**
 * The directory's explicit sort control: a strip of the three orders where
 * there is room for them, and a chip that opens a sheet where there is not.
 *
 * It states the same fact as the table's sortable column headers and writes the
 * same param. Both exist because they answer at different widths and in
 * different registers: the headers are the in-place control for a reader
 * already scanning the columns, and they are not drawn at all below `sm`, where
 * the table has no header row and this chip is the only sort affordance.
 */
export function VenueSortControl({
  sort,
  onSortChange,
  className,
}: VenueSortControlProps) {
  const [open, setOpen] = useState(false)

  return (
    <div className={className}>
      {/* The strip. Hidden below `sm`, where the sheet below takes over. */}
      <div
        className="hidden items-center gap-2 text-sm sm:flex"
        data-testid="venue-sort-strip"
      >
        <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Sort
        </span>
        {VENUE_SORTS.map((key, index) => (
          <span key={key} className="inline-flex items-center gap-2">
            {index > 0 && (
              <span aria-hidden="true" className="text-muted-foreground/50">
                &middot;
              </span>
            )}
            <button
              type="button"
              onClick={() => onSortChange(key)}
              aria-pressed={sort === key}
              data-testid={`venue-sort-option-${key}`}
              className={cn(
                'inline-flex items-center gap-1 hover:text-foreground',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                sort === key ? 'text-primary' : 'text-muted-foreground'
              )}
            >
              {VENUE_SORT_LABELS[key]}
              {sort === key && <span aria-hidden="true">&#9662;</span>}
            </button>
          </span>
        ))}
      </div>

      {/* The chip. A sheet rather than a popover on a narrow viewport, the same
          call the city filter makes there. */}
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>
          <button
            // In server HTML and clickable before React attaches Radix's
            // handler, so a press in that window is replayed rather than
            // dropped.
            {...replayOnHydrate}
            type="button"
            data-testid="venue-sort-chip"
            className={cn(
              'inline-flex min-h-11 items-center gap-1.5 rounded-md border border-border/50 bg-muted/50 px-3 text-sm',
              'hover:border-border hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'sm:hidden'
            )}
          >
            Sort: {VENUE_SORT_LABELS[sort]}
            <span aria-hidden="true">&#9662;</span>
          </button>
        </SheetTrigger>
        <SheetContent
          side="bottom"
          showCloseButton={false}
          className="gap-0 rounded-t-lg p-0"
          data-testid="venue-sort-sheet"
        >
          <div className="flex items-center justify-between gap-2 px-4 pb-2 pt-4">
            <SheetTitle className="text-base">Sort rooms by</SheetTitle>
            <SheetDescription className="sr-only">
              Pick the order the rooms are listed in.
            </SheetDescription>
            <SheetClose
              className="-my-1 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              data-testid="venue-sort-sheet-close"
            >
              Close
            </SheetClose>
          </div>
          <div role="group" aria-label="Sort rooms by" className="px-2 pb-4">
            {VENUE_SORTS.map(key => (
              <button
                key={key}
                type="button"
                aria-pressed={sort === key}
                data-testid={`venue-sort-sheet-option-${key}`}
                onClick={() => {
                  onSortChange(key)
                  setOpen(false)
                }}
                className={cn(
                  'flex min-h-11 w-full items-center gap-2 rounded-md px-3 text-left text-sm',
                  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  sort === key && 'bg-secondary font-medium'
                )}
              >
                <Check
                  className={cn(
                    'h-4 w-4 shrink-0',
                    sort === key ? 'opacity-100' : 'opacity-0'
                  )}
                  aria-hidden="true"
                />
                {VENUE_SORT_LABELS[key]}
              </button>
            ))}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
