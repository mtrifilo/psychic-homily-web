import Link from 'next/link'

/**
 * The map link's type treatment: underlined body copy hovering to `foreground`
 * rather than the `primary` of the prose links elsewhere. Exported because the
 * scene section offers the same target from its desktop foot, where the
 * surrounding copy and spacing differ but the link should not.
 */
export const GRAPH_MAP_LINK_CLASS = 'underline underline-offset-4 hover:text-foreground'

/**
 * MobileGraphTeaser — the sub-`GRAPH_BREAKPOINT_PX` form of a graph section.
 *
 * No graph canvas renders below the breakpoint on any surface, so whatever the
 * section puts in that slot is the entire section at that width. This is that
 * slot, and it is one line of body text whose sentence is the cross-link into
 * the knowledge graph: no heading, no scale line, no reserved box.
 *
 * The caller supplies the sentence and the href; the trailing arrow and the
 * type/link treatment belong to the component, so the surfaces that share it
 * cannot drift apart on the glyph, the spacing, or the underline.
 *
 * The arrow is decorative: the sentence alone is the accessible name.
 */
export function MobileGraphTeaser({
  href,
  children,
}: {
  href: string
  children: string
}) {
  return (
    <p className="text-xs text-muted-foreground">
      <Link href={href} className={GRAPH_MAP_LINK_CLASS}>
        {children} <span aria-hidden="true">→</span>
      </Link>
    </p>
  )
}
