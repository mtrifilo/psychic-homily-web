import { cn } from '@/lib/utils'

/** Known relationship types between entities */
export type RelationshipType =
  | 'label-mate'
  | 'similar'
  | 'shared-bills'
  | 'side-project'
  | 'member-of'
  | 'formerly'

/** Display labels for relationship types */
const RELATIONSHIP_LABELS: Record<RelationshipType, string> = {
  'label-mate': 'label mate',
  similar: 'similar',
  'shared-bills': 'shared bills',
  'side-project': 'side project',
  'member-of': 'member of',
  formerly: 'formerly',
}

export interface RelationshipBadgeProps {
  /** The type of relationship */
  type: RelationshipType
  /** Optional custom label override */
  label?: string
  /** Optional count to display, e.g. "shared 3 bills" */
  count?: number
  /** Additional CSS classes */
  className?: string
}

/**
 * A small inline badge indicating the relationship between two entities.
 * Used on entity detail pages and cards to show connections in the knowledge graph.
 *
 * Usage:
 *   <RelationshipBadge type="label-mate" />
 *   <RelationshipBadge type="shared-bills" count={3} />
 *   <RelationshipBadge type="side-project" label="side project of" />
 */
export function RelationshipBadge({
  type,
  label,
  count,
  className,
}: RelationshipBadgeProps) {
  const displayLabel = label ?? RELATIONSHIP_LABELS[type]

  // Format the label with count if applicable
  const formattedLabel =
    type === 'shared-bills' && count !== undefined
      ? `shared ${count} bill${count !== 1 ? 's' : ''}`
      : displayLabel

  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md px-1.5 py-0.5',
        'text-[11px] font-medium leading-none',
        'bg-secondary text-secondary-foreground/70',
        'border border-border/30',
        className
      )}
    >
      {formattedLabel}
    </span>
  )
}
