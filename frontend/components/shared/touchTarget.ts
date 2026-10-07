/**
 * Grows a button's tap target to at least 24x24 CSS px (the WCAG 2.5.8
 * minimum target size) on coarse pointers, without changing anything visible.
 *
 * The extra area is a transparent `::after` box centred on the control's
 * padding box: as large as that box, and never smaller than 24px on either
 * axis. A tap on it is a tap on the control. It takes no space in layout and
 * paints nothing, so on a button host the glyph, the border, the focus ring
 * and the surrounding layout are identical on every pointer type. Every
 * utility is gated on `pointer-coarse`, so fine pointers get none of it.
 *
 * Host requirements:
 * - A `<button>`. Chromium draws the `outline: auto` keyboard focus ring of
 *   other block-level hosts, such as a `flex` link, around their positioned
 *   descendants, so on those the ring grows to the hit area on a coarse
 *   pointer. A button's ring stays on its own box.
 * - Not already positioned (`absolute`, `fixed`, `sticky`): `relative`
 *   replaces that positioning on coarse pointers, and its `::after` is free.
 * - No overflow clipping on the host (`overflow-hidden`, `truncate`, a scroll
 *   container) or on an ancestor flush against it, or the grown box is
 *   clipped back and the target does not grow.
 * - Neighbours far enough away that the grown box does not cover them: the
 *   box reaches `(24 - padding-box size) / 2` px past each undersized edge.
 *
 * Where the control's visible box may grow instead, `ATLAS_LINK_TARGET_CLASS`
 * sets a 24px minimum height on the control itself.
 */
export const COARSE_POINTER_HIT_AREA_CLASS =
  'pointer-coarse:relative pointer-coarse:after:absolute pointer-coarse:after:top-1/2 pointer-coarse:after:left-1/2 pointer-coarse:after:size-full pointer-coarse:after:min-h-6 pointer-coarse:after:min-w-6 pointer-coarse:after:-translate-1/2'
