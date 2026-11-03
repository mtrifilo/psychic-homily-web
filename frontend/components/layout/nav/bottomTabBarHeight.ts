/**
 * Height of the fixed mobile tab bar, or 0 where it is not rendered (it is
 * display:none from `xl`, which measures as 0).
 *
 * For overlays positioned in JS, which cannot read back the
 * `--bottom-tab-bar-height` calc() as a number. Read it at positioning time,
 * not once at mount, so a viewport resize across `xl` is honoured. Browser
 * only.
 */
export function bottomTabBarHeight(): number {
  const bar = document.querySelector<HTMLElement>('[data-bottom-tab-bar]')
  return bar?.getBoundingClientRect().height ?? 0
}
