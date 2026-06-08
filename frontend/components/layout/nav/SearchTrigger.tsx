'use client'

import { Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'

// The dominant search action: a field-styled button that opens the existing
// CommandPalette (⌘K). It is presented as the primary right-hand affordance per
// the Figma design. The palette itself is unchanged here (its re-skin is
// PSY-1019).
export function SearchTrigger({ className }: { className?: string }) {
  return (
    <button
      type="button"
      onClick={() => openCommandPalette()}
      aria-label="Search shows, artists, labels"
      aria-keyshortcuts="Meta+K Control+K"
      className={cn(
        'flex h-9 w-[220px] items-center gap-2 rounded-lg border border-input bg-muted px-3 text-left text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 xl:w-[320px]',
        className
      )}
    >
      <Search className="size-4 shrink-0" aria-hidden />
      <span className="flex-1 truncate">Search shows, artists, labels…</span>
      <kbd className="pointer-events-none inline-flex shrink-0 items-center rounded border border-input bg-background px-1.5 font-mono text-[11px] text-muted-foreground">
        ⌘K
      </kbd>
    </button>
  )
}
