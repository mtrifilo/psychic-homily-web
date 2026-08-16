'use client'

import { Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'

// The dominant search action: a field-styled button that opens the existing
// CommandPalette (⌘K). It is presented as the primary right-hand affordance per
// the Figma design. The palette itself is unchanged here (its re-skin is
// PSY-1019).
//
// WIDTH belongs to the CALLER, CHROME belongs to this component, and the two
// meet at one shared breakpoint. The trigger used to carry `w-[220px]
// xl:w-[320px]` itself, which made it an unshrinkable fixed-width island in the
// top bar's flex row and pushed the account cluster off-screen (PSY-1638), so
// it fills whatever box it is given (`w-full`) and the caller sizes that box.
// Its own children degrade gracefully at any width — the icon and ⌘K hint are
// `shrink-0` and the label truncates.
//
// Below `sm` it condenses to a bare icon tap target (PSY-1020 — search stays
// reachable on phones, where the top bar has no room for field chrome). That is
// a responsive form of THIS button, not a second control (PSY-1818): the top
// bar used to render a forked icon-only button beside this one, so both nodes
// were always in the DOM with two different accessible names ("Search" vs
// "Search shows, artists, labels") and only CSS deciding which one a user or a
// test could see. One node, one name, every width.
//
// CONTRACT WITH THE CALLER: the caller's box must switch to the field width at
// the SAME `sm` breakpoint the chrome below switches at. Move one without the
// other and there is a viewport band that renders field chrome (border, px-3,
// a shrink-0 ⌘K) inside a 36px box, or a 220px box holding a centred bare icon.
// Deliberately NOT a container query: PSY-1638 has the caller's box SHRINK
// below its nominal width when the nav row is crowded, so keying the chrome off
// the box's own width would collapse a merely-narrowed desktop field to the
// phone icon.
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
        // Compact form. The hover trio mirrors buttonVariants' `ghost` exactly
        // (incl. its dark half) — this replaced a <Button variant="ghost"
        // size="icon"> and must not lose its dark-mode hover. Not composed from
        // buttonVariants itself: that cva base carries font-semibold and
        // rounded-md, which fight the field form below.
        'flex h-9 w-full items-center justify-center rounded-lg text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
        // Field form. Both hover backgrounds are re-stated rather than left to
        // cancel by variant ordering: the field's background does not change on
        // hover, only its text colour does.
        'sm:justify-start sm:gap-2 sm:border sm:border-input sm:bg-muted sm:px-3 sm:text-left sm:hover:bg-muted sm:hover:text-foreground sm:dark:hover:bg-muted',
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
