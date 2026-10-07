/**
 * The one import of the GlobeCanvas module. AtlasGlobe's next/dynamic and
 * preloadAtlasMap both call it, so the bundler emits one chunk group for the
 * canvas and the preload's downloads are the ones the render then uses. A
 * second `import('./GlobeCanvas')` anywhere, including an inline one inside
 * next/dynamic, gets its own copy of the group's entry chunk.
 */
export function loadGlobeCanvas() {
  return import('./GlobeCanvas')
}
