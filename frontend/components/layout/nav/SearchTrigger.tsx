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
//
// Below `sm` it condenses to a bare icon tap target (PSY-1020 — search stays
// reachable on phones, where the top bar has no room for field chrome). That is
// a responsive form of THIS button, not a second control (PSY-1818): the top
// bar used to render a forked icon-only button beside this one, so both nodes
// were always in the DOM with two different accessible names ("Search" vs
// "Search shows, artists, labels") and only CSS deciding which one a user or a
// test could see. One node, one name, every width.
//
// Deliberately NO replayOnHydrate on this button: the CommandPalette's
// open-event listener registers in a passive effect that flushes AFTER the
// hydration-commit replay would fire, so a replayed tap would dispatch into an
// empty listener set AND consume the buffered click — worse than dropping it.
// Adding it here (or to any other palette trigger) requires moving that
// listener to useLayoutEffect or module scope first.
export function SearchTrigger({ className }: { className?: string }) {
  return (
    <button
      type="button"
      onClick={() => openCommandPalette()}
      aria-label="Search shows, artists, labels"
      aria-keyshortcuts="Meta+K Control+K"
      className={cn(
        'flex h-9 w-full items-center justify-center rounded-lg text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
        'sm:justify-start sm:gap-2 sm:border sm:border-input sm:bg-muted sm:px-3 sm:text-left sm:hover:bg-muted sm:hover:text-foreground',
        className
      )}
    >
      <Search className="size-5 shrink-0 sm:size-4" aria-hidden />
      <span className="hidden flex-1 truncate sm:block">Search shows, artists, labels…</span>
      <kbd className="pointer-events-none hidden shrink-0 items-center rounded border border-input bg-background px-1.5 font-mono text-[11px] text-muted-foreground sm:inline-flex">
        ⌘K
      </kbd>
    </button>
  )
}
