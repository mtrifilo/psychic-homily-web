import { cn } from '@/lib/utils'

/**
 * Single-sourced entity-type → DS-palette color map for the six core
 * knowledge-graph entity types (PSY-943). Bound to the shared categorical
 * `--chart-1..8` tokens (globals.css, PSY-947) so badge hues track the
 * newsprint/vinyl theme in both modes instead of drifting off-palette with
 * raw Tailwind `blue-500`/`purple-500`/etc.
 *
 * Hue assignment is the interleaved warm/cool subset the analytics dashboard
 * already uses for its entity-creation chart (AnalyticsDashboard `COLORS`):
 * orange · denim · gold · plum · green · teal. Interleaving warm and cool
 * keeps adjacent types distinguishable at a glance without the full rainbow.
 *
 * `--chart-4` is intentionally absent here: it equals `--destructive` in light
 * mode (`#9c2a1a`), so reusing it for a neutral entity type would read as an
 * error tone. Festivals take `--chart-8` (teal) instead.
 *
 * Tint convention matches the rest of the badge surfaces: a low-alpha token
 * background, full-strength token text, and a mid-alpha token border. The
 * full-strength text keeps adequate contrast against the muted tint in both
 * light and dark modes (the chart tokens are mid-saturation, not pastel).
 */
const ENTITY_TYPE_BADGE_CLASSES: Record<string, string> = {
  artist: 'bg-chart-1/15 text-chart-1 border-chart-1/30',
  venue: 'bg-chart-6/15 text-chart-6 border-chart-6/30',
  show: 'bg-chart-3/15 text-chart-3 border-chart-3/30',
  release: 'bg-chart-7/15 text-chart-7 border-chart-7/30',
  label: 'bg-chart-2/15 text-chart-2 border-chart-2/30',
  festival: 'bg-chart-8/15 text-chart-8 border-chart-8/30',
}

const ENTITY_TYPE_BADGE_FALLBACK =
  'bg-muted text-muted-foreground border-border'

/**
 * Returns the DS-palette tint classes for an entity type, or the muted
 * fallback for an unknown type. Use this when a caller renders its own badge
 * element (e.g. an existing custom span with extra layout classes) and only
 * needs the color slice — it keeps every surface on the SAME map as
 * {@link EntityTypeBadge}.
 */
export function getEntityTypeBadgeClasses(type: string): string {
  return ENTITY_TYPE_BADGE_CLASSES[type] ?? ENTITY_TYPE_BADGE_FALLBACK
}

export interface EntityTypeBadgeProps {
  /** One of artist/venue/show/release/label/festival (others render muted). */
  type: string
  /**
   * Override the visible text. Defaults to the raw `type` string so existing
   * call sites that showed the lowercase type keep their output unchanged.
   */
  label?: string
  className?: string
  testId?: string
}

/**
 * Small colored pill marking a knowledge-graph entity's type. The hue is
 * single-sourced via {@link getEntityTypeBadgeClasses} so admin tables, the
 * request board, and any future surface share one palette.
 */
export function EntityTypeBadge({
  type,
  label,
  className,
  testId,
}: EntityTypeBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium',
        getEntityTypeBadgeClasses(type),
        className
      )}
      data-testid={testId}
    >
      {label ?? type}
    </span>
  )
}
