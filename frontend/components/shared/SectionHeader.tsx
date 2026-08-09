'use client'

import { cn } from '@/lib/utils'

export interface SectionHeaderProps {
  /** Section title (rendered in uppercase tracking-wider style). */
  title: string
  /** Optional inline action node, rendered to the right of the title. Typically a <BracketLink>. */
  action?: React.ReactNode
  /** Heading level for the rendered element. Defaults to 'h3'. */
  as?: 'h2' | 'h3' | 'h4'
  /** Visual size: 'sm' for sidebar (11–12px), 'md' for main column (13–14px). Defaults to 'sm'. */
  size?: 'sm' | 'md'
  /**
   * Title treatment. 'caps' (default) is the dense uppercase-tracking style
   * used across entity pages; 'title' is the title-case foreground style the
   * profile redesign boards use for main-column sections (PSY-1062).
   */
  variant?: 'caps' | 'title'
  /** Render a thin underline divider beneath the header. Defaults to true. */
  underline?: boolean
  /**
   * Extra props forwarded to the heading element itself, for callers that need
   * to address it directly rather than the wrapper — chiefly moving focus to it
   * after a client-side list update, where `usePaginationFocusTarget()`'s
   * `targetProps` (a ref plus `tabIndex: -1`) spreads straight in.
   *
   * Deliberately narrow: this is not a general escape hatch for restyling the
   * heading, which is what `size`/`variant`/`className` are for.
   */
  headingProps?: Pick<
    React.ComponentPropsWithRef<'h2'>,
    'ref' | 'tabIndex' | 'id'
  >
  /** Additional CSS classes on the wrapping div. */
  className?: string
}

/**
 * Tight section header for density-first surfaces. One-line bar with a title and an
 * optional inline action (commonly a <BracketLink> like [Toggle] or [Add tag]).
 *
 * State (collapsed / expanded) is owned by the parent — this primitive is presentational
 * only. The parent decides whether to render the section body based on its own state.
 *
 * Usage:
 *   <SectionHeader title="Statistics" />
 *
 *   <SectionHeader
 *     title="Past shows"
 *     action={<BracketLink label={collapsed ? 'Show' : 'Hide'} onClick={() => setCollapsed(c => !c)} />}
 *   />
 *
 *   <SectionHeader title="Tags" action={<BracketLink label="Add tag" onClick={openTagForm} />} />
 */
export function SectionHeader({
  title,
  action,
  as: Tag = 'h3',
  size = 'sm',
  underline = true,
  variant = 'caps',
  headingProps,
  className,
}: SectionHeaderProps) {
  return (
    <div
      className={cn(
        'flex items-baseline justify-between gap-2',
        underline && 'border-b border-border/50 pb-1 mb-2',
        className
      )}
    >
      <Tag
        {...headingProps}
        className={cn(
          'font-semibold',
          // A programmatically focused heading gets a visible ring; without it
          // a sighted keyboard user is left with no idea where focus landed.
          headingProps?.tabIndex === -1 &&
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          variant === 'caps' && 'text-muted-foreground uppercase tracking-wider',
          variant === 'caps' && size === 'sm' && 'text-xs',
          variant === 'caps' && size === 'md' && 'text-sm',
          variant === 'title' && 'text-foreground',
          variant === 'title' && size === 'sm' && 'text-sm',
          variant === 'title' && size === 'md' && 'text-base'
        )}
      >
        {title}
      </Tag>
      {action && <div className="shrink-0 text-xs">{action}</div>}
    </div>
  )
}
