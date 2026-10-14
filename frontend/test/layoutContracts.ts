// Layout contracts that span more than one component, expressed once so the
// tests on either side of a contract cannot drift apart.
//
// A contract lives here when two components have to agree on a literal but have
// no way to share it in source: Tailwind scans raw text, so a class name built
// from a runtime constant never gets generated. That forces the literal to be
// duplicated in the components — which is exactly why the TESTS need a single
// shared copy. Change the expression in one component only, and the other
// component's test fails against this constant instead of quietly passing.

// The bottom tab bar's full occupied band below `xl`: the bar's own height plus
// the iOS home-indicator inset (PSY-1020 bar, PSY-1820 safe area).
// --bottom-tab-bar-height already includes the bar's 1px border-t, so this is
// the whole fixed element.
//
// BottomTabBar RENDERS it as its border-box height (`h-[…]`); AppShell RESERVES
// it inside its bottom padding (`pb-[calc(<box>+<banner>)]`, the box nested
// whole so it stays a literal substring). The two must stay byte-identical or
// page content slides under the bar or floats above a gap.
export const BOTTOM_TAB_BAR_BOX =
  'calc(var(--bottom-tab-bar-height)+env(safe-area-inset-bottom))'

// The cookie consent banner's height. CookieConsentBanner PUBLISHES it on
// <html> under this name only while the banner is up; AppShell's bottom
// padding RESERVES it, and AtlasGlobe's frame height and minimum SUBTRACT it.
// The readers spell it with a 0px fallback (COOKIE_BANNER_HEIGHT), so a
// rename on one side alone breaks nothing visibly at runtime: the publisher's
// test and every reader's test assert against these two constants instead.
export const COOKIE_BANNER_HEIGHT_VAR = '--cookie-banner-height'
export const COOKIE_BANNER_HEIGHT = `var(${COOKIE_BANNER_HEIGHT_VAR},0px)`
