/**
 * The two treatments an internal entity link takes on the artist and show
 * pages.
 *
 * - `accent`: the link IS the next thing to read. Discovery lists (similar
 *   artists, a show's support acts) and the scene link under an artist's name.
 *   Accent colour at rest, full accent on hover, medium weight.
 * - `restrained`: a name inside a dense table, where accent on every row
 *   would turn the table orange. Colour and weight at rest come from the
 *   surrounding text, so a bill's headliner/support hierarchy survives;
 *   accent and underline arrive on hover.
 *
 * Opt-in per call site: a surface not listed above keeps its own styling
 * until it is migrated deliberately.
 *
 * No directive and no client-only imports, so a server module can import
 * these strings as values rather than as client references.
 */
export type EntityLinkTier = 'accent' | 'restrained'

export const ENTITY_LINK_CLASS: Readonly<Record<EntityLinkTier, string>> = {
  accent: 'font-medium text-primary/80 transition-colors hover:text-primary',
  restrained:
    'underline-offset-2 transition-colors hover:text-primary hover:underline',
}
