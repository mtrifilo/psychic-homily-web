import { HomeDateline } from './HomeDateline'
import {
  ScryingGridWordmark,
  WORDMARK_ONE_LINE,
} from './scrying-grid/ScryingGridWordmark'

/**
 * The signed-in home's masthead, above the sections (Product Designs Home,
 * Concept 1 "Nameplate": phone 2241:3317, desktop 2242:3702).
 *
 * Below `xl` the top bar carries the wordmark, so this is the short dateline
 * alone. From `xl` it is the one-line wordmark at nameplate size, the long
 * dateline centred beneath, and a hairline. It scrolls with the page; it is
 * not sticky. The wordmark is decorative here (`heading={false}`): the page's
 * h1 is the runtime's.
 *
 * Isomorphic: renders under the server's SignedInHome and under the client
 * fallback alike.
 */
export function HomeNameplate() {
  // pb: the board's gap down to the first section (20px under the phone
  // dateline, 32px under the desktop hairline).
  return (
    <div className="pb-5 xl:pb-8">
      <HomeDateline form="short" className="pt-3 xl:hidden" />
      <div className="hidden pt-8 xl:block">
        <ScryingGridWordmark
          preset="nameplate"
          lines={WORDMARK_ONE_LINE}
          heading={false}
          motion="settle"
          className="mx-auto max-w-full"
        />
        <HomeDateline form="long" className="mt-3.5 text-center" />
        <div aria-hidden className="mt-4 h-px w-full bg-border" />
      </div>
    </div>
  )
}
