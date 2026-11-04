import { HomeDateline } from './HomeDateline'
import { ScryingGridWordmark } from './scrying-grid/ScryingGridWordmark'
// From the isomorphic module, not the client one: this file also renders on
// the server.
import { WORDMARK_ONE_LINE } from './scrying-grid/wordmarkPresets'

/**
 * The signed-in home's masthead, above the sections (Product Designs Home,
 * Concept 1 "Nameplate": phone 2241:3317, desktop 2242:3702).
 *
 * Below `sm` the top bar carries the wordmark (TopBar's `sm:hidden` mark is
 * the other half of this file's `sm` split; change them together), so this is
 * the short dateline alone. From `sm` it is the one-line wordmark at nameplate
 * size, scaled to the content column (100px tall from a 1429px viewport), the
 * long dateline centred beneath, and a hairline. It scrolls with the page; it
 * is not sticky. The wordmark is decorative here (`heading={false}`): the
 * page's h1 is the runtime's.
 *
 * Isomorphic: renders under the server's SignedInHome and under the client
 * fallback alike.
 */
export function HomeNameplate() {
  // pb: the board's gap down to the first section (20px under the phone
  // dateline, 32px under the hairline).
  //
  // The datelines arrive after hydration, so where a line can wrap its second
  // row is reserved up front and the arriving text cannot push the rendered
  // rows down. Measured in the data face with five-digit show and six-digit
  // entity counts: the short line is up to about 398px (wraps below a 430px
  // viewport, 12px padding + 2 x 16px rows), the long one up to about 631px
  // (wraps below about 670px, 2 x 18px rows).
  return (
    <div className="pb-5 sm:pb-8">
      <HomeDateline form="short" className="pt-3 max-[429px]:min-h-11 sm:hidden" />
      <div className="hidden pt-8 sm:block">
        <ScryingGridWordmark
          preset="nameplate"
          lines={WORDMARK_ONE_LINE}
          heading={false}
          motion="settle"
          className="mx-auto h-[clamp(56px,7vw,100px)] max-w-full"
        />
        <HomeDateline
          form="long"
          className="mt-3.5 text-center max-[669px]:min-h-9"
        />
        <div aria-hidden className="mt-4 h-px w-full bg-border" />
      </div>
    </div>
  )
}
