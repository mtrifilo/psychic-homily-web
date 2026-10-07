/**
 * The one import of the GlobeCanvas module. AtlasGlobe's next/dynamic and
 * preloadAtlasMap both call it, so the bundler emits one chunk group for the
 * canvas and the preload's downloads are the ones the render then uses. A
 * second `import('./GlobeCanvas')` anywhere, including an inline one inside
 * next/dynamic, gets a chunk group of its own, and the visitor downloads
 * whatever the bundler puts in both groups twice.
 */
export function loadGlobeCanvas() {
  return import('./GlobeCanvas')
}
