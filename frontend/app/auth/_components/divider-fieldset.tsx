import type { ReactNode } from 'react'

/**
 * A group of form controls named by a legend drawn as a labelled divider:
 * a rule, the legend in the kicker style, a rule. The legend is the group's
 * accessible name, so its text stays sentence case and `uppercase` styles it.
 *
 * Clearance is 24px above and below the divider row when the fieldset sits in
 * a `space-y-4` stack: above, the stack's 16px gap plus the legend's 8px top
 * padding; below, the legend's 8px bottom padding plus the group's own 16px
 * top margin. The rules are borders, not backgrounds, so forced-colors mode
 * still paints them.
 */
export function DividerFieldset({
  legend,
  children,
}: {
  legend: string
  children: ReactNode
}) {
  return (
    <fieldset className="min-w-0">
      <legend className="w-full py-2">
        <span className="flex items-center gap-2">
          <span aria-hidden="true" className="flex-1 border-t border-separator" />
          <span className="shrink-0 font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
            {legend}
          </span>
          <span aria-hidden="true" className="flex-1 border-t border-separator" />
        </span>
      </legend>
      <div className="mt-4 space-y-4">{children}</div>
    </fieldset>
  )
}
