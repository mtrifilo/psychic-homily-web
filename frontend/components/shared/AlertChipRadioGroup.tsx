'use client'

import { useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export interface AlertChipOption<T extends string> {
  value: T
  label: string
}

interface AlertChipRadioGroupProps<T extends string> {
  /** Accessible name for the group. Several on a page must stay distinct. */
  ariaLabel: string
  /** Visible lead-in, e.g. "Alerts:". */
  label: string
  options: readonly AlertChipOption<T>[]
  value: T
  onChange: (value: T) => void
  /** Parks the whole group while a write is in flight. */
  pending?: boolean
  className?: string
}

/**
 * The outlined-chip radio group used by every post-follow alert control.
 *
 * Generalized from the scene follow's notify-mode toggle (PSY-1905) so the new
 * artist/venue scope reveal shares its markup and ARIA contract rather than
 * shipping a second copy of both. Note the consequence for the shipped
 * control: the scene toggle GAINED roving tabindex and arrow navigation here,
 * which it did not have before.
 *
 * Deliberately presentational: it owns no query and no mutation, so the two
 * callers keep their own (genuinely different) storage and optimistic-update
 * paths. What it owns is the markup and the ARIA contract.
 *
 * Not exported from `components/shared/index.ts` on purpose. See the barrel
 * guard in `features/sharedChunkBarrelGuard.test.ts`.
 */
export function AlertChipRadioGroup<T extends string>({
  ariaLabel,
  label,
  options,
  value,
  onChange,
  pending = false,
  className,
}: AlertChipRadioGroupProps<T>) {
  const chipRefs = useRef<(HTMLButtonElement | null)[]>([])
  const selectedIndex = options.findIndex(option => option.value === value)
  // Focus has to land somewhere when nothing is selected yet, or the group
  // becomes unreachable by keyboard entirely.
  const [focusIndex, setFocusIndex] = useState(Math.max(selectedIndex, 0))

  // Clamped at RENDER, not just on write, because the option list can shrink
  // underneath a stored index. An artist with a home area offers three chips;
  // clearing that area (in this tab or another, via the shared account
  // preferences cache) drops it to two. A focusIndex of 2 then matches no chip,
  // every chip gets tabIndex={-1}, and the group falls out of the tab order
  // entirely: the single tab stop this roving-tabindex contract promises stops
  // existing. Deriving it keeps the invariant true for any option list.
  const activeFocusIndex = Math.min(focusIndex, options.length - 1)

  const commit = (index: number) => {
    const option = options[index]
    if (!option || pending || option.value === value) return
    onChange(option.value)
  }

  // Arrows MOVE, they do not commit.
  //
  // Selection-following-focus is the tempting shape and it is wrong here,
  // twice over. Every keystroke would fire a PATCH, and arrowing past a chip
  // on the way to the one you want would STORE it: passing through
  // "Everywhere" on an artist overwrites the near-me scope the read path goes
  // out of its way to preserve. It also collides with `pending` below: a
  // commit on arrow disables the group mid-keystroke, and a disabled button
  // cannot hold focus, so the browser drops it to <body> and the next Tab
  // restarts from the top of the document.
  const moveFocus = (index: number) => {
    const next = (index + options.length) % options.length
    setFocusIndex(next)
    chipRefs.current[next]?.focus()
  }

  const handleKeyDown = (
    event: React.KeyboardEvent<HTMLButtonElement>,
    index: number
  ) => {
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        event.preventDefault()
        moveFocus(index + 1)
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        event.preventDefault()
        moveFocus(index - 1)
        break
      case 'Home':
        event.preventDefault()
        moveFocus(0)
        break
      case 'End':
        event.preventDefault()
        moveFocus(options.length - 1)
        break
      case ' ':
      case 'Enter':
        event.preventDefault()
        commit(index)
        break
    }
  }

  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={cn('flex flex-wrap items-center gap-1', className)}
    >
      <span className="text-muted-foreground">{label}</span>
      {options.map((option, index) => (
        <button
          key={option.value}
          ref={node => {
            chipRefs.current[index] = node
          }}
          type="button"
          role="radio"
          aria-checked={value === option.value}
          tabIndex={index === activeFocusIndex ? 0 : -1}
          // aria-disabled, NOT disabled: a disabled button cannot hold focus,
          // so parking the group during a write would eject the keyboard user
          // out of it. The guard in `commit` is what actually blocks the write.
          aria-disabled={pending || undefined}
          onKeyDown={event => handleKeyDown(event, index)}
          onFocus={() => setFocusIndex(index)}
          onClick={() => commit(index)}
          className={cn(
            'rounded-full border px-2 py-0.5 transition-colors',
            value === option.value
              ? 'border-primary text-foreground'
              : 'border-border text-muted-foreground hover:border-primary/60 hover:text-foreground',
            pending && 'cursor-not-allowed opacity-60'
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
