/**
 * What the page around the map can read about the map credit (the MapLibre
 * attribution control GlobeCanvas mounts). The selectors here are the one
 * place outside GlobeCanvas and globals.css that knows the control's markup;
 * kept out of GlobeCanvas itself so reading them never pulls MapLibre into a
 * caller's bundle.
 */

/**
 * Whether `root` holds a top-left map credit that is drawn. MapLibre keeps
 * its attribution control mounted but hidden (`maplibregl-attrib-empty`)
 * while no source in use carries an attribution, and there is no control at
 * all before the map has mounted its controls. GlobeCanvas marks the
 * top-left placement with `data-atlas-credit="top"`.
 */
export function atlasTopCreditShown(root: ParentNode): boolean {
  const credit = root.querySelector(
    "[data-atlas-credit='top'] .maplibregl-ctrl-attrib",
  )
  return (
    credit !== null && !credit.classList.contains('maplibregl-attrib-empty')
  )
}
