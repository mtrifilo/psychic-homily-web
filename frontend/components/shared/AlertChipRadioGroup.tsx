'use client'

import { useRef } from 'react'
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
 * Extracted from two byte-identical copies (PSY-1905): the scene follow's
 * notify-mode toggle and the artist/venue follow's scope reveal. They are the
 * same control over the same idea, so a style or accessibility fix to one
 * should never have needed applying twice in two features with nothing linking
 * them.
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

  // A radiogroup is ONE tab stop whose members are reached with the arrow
  // keys. Leaving every chip independently focusable announces "radio 1 of 3"
  // and then refuses to navigate the way it just promised, which is worse than
  // not claiming the role. Home/End included because the pattern specifies
  // them and they are free once focus is already managed.
  const moveTo = (index: number) => {
    const next = (index + options.length) % options.length
    const option = options[next]
    if (!option) return
    chipRefs.current[next]?.focus()
    if (option.value !== value) onChange(option.value)
  }

  const handleKeyDown = (
    event: React.KeyboardEvent<HTMLButtonElement>,
    index: number
  ) => {
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        event.preventDefault()
        moveTo(index + 1)
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        event.preventDefault()
        moveTo(index - 1)
        break
      case 'Home':
        event.preventDefault()
        moveTo(0)
        break
      case 'End':
        event.preventDefault()
        moveTo(options.length - 1)
        break
    }
  }

  // Focus has to land somewhere when nothing is selected yet, or the group
  // becomes unreachable by keyboard entirely.
  const focusedIndex = Math.max(
    options.findIndex(option => option.value === value),
    0
  )

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
          tabIndex={index === focusedIndex ? 0 : -1}
          disabled={pending}
          onKeyDown={event => handleKeyDown(event, index)}
          onClick={() => {
            if (value !== option.value) onChange(option.value)
          }}
          className={cn(
            'rounded-full border px-2 py-0.5 transition-colors',
            value === option.value
              ? 'border-primary text-foreground'
              : 'border-border text-muted-foreground hover:border-primary/60 hover:text-foreground',
            pending && 'opacity-60'
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
