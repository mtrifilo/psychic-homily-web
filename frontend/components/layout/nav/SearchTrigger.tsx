'use client'

import { Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'

// The dominant search action: a field-styled button that opens the existing
// CommandPalette (⌘K). It is presented as the primary right-hand affordance per
// the Figma design. The palette itself is unchanged here (its re-skin is
// PSY-1019).
//
// Sizing belongs to the CALLER, not to this component: it fills whatever box it
// is given (`w-full`). The trigger used to carry `w-[220px] xl:w-[320px]`
// itself, which made it an unshrinkable fixed-width island in the top bar's
// flex row and pushed the account cluster off-screen (PSY-1638). Its own
// children already degrade gracefully at any width — the icon and ⌘K hint are
// `shrink-0` and the label truncates — so the container can size it freely.
export function SearchTrigger({ className }: { className?: string }) {
  return (
    <button
      type="button"
      onClick={() => openCommandPalette()}
      aria-label="Search shows, artists, labels"
      aria-keyshortcuts="Meta+K Control+K"
      className={cn(
        'flex h-9 w-full items-center gap-2 rounded-lg border border-input bg-muted px-3 text-left text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
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
