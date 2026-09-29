/**
 * The two treatments an internal entity link takes on the artist and show
 * pages.
 *
 * - `accent`: the link IS the next thing to read. Used by the similar-artists
 *   list, a show page's support acts, the scene link under an artist's name,
 *   and the venue name in the show venue module. The `link` colour token at
 *   rest and on hover, medium weight, underline on hover. `link` clears WCAG
 *   AA on the page and card backgrounds only, so an accent link never sits on
 *   a muted, secondary, sidebar or accent surface, at rest or on hover.
 * - `restrained`: a name inside a dense table, where accent on every row
 *   would turn the table orange. Used by the artist page's shows tables
 *   (upcoming and past) through ShowBill's opt-in. Colour and weight at rest
 *   come from the surrounding text, so a bill's headliner/support hierarchy
 *   survives; accent and underline arrive on hover.
 *
 * Opt-in per call site: a surface not listed above keeps its own styling
 * until it is migrated deliberately.
 *
 * No directive and no client-only imports, so a server module can import
 * these strings as values rather than as client references.
 */
export type EntityLinkTier = 'accent' | 'restrained'

export const ENTITY_LINK_CLASS: Readonly<Record<EntityLinkTier, string>> = {
  accent: 'font-medium text-link underline-offset-2 hover:underline',
  restrained:
    'underline-offset-2 transition-colors hover:text-primary hover:underline',
}
