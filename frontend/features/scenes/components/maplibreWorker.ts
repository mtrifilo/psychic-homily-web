// maplibre-gl v6 has NO default export. `import maplibregl from 'maplibre-gl'`
// is `undefined` and fails confusingly, so namespace import only.
import * as maplibregl from 'maplibre-gl'

// PSY-1537 SPIKE FINDING (load-bearing): Turbopack rewrites maplibre's
// `import.meta.url` to a file://…node_modules… URL, so v6's runtime worker
// resolution returns "" and the map hangs FOREVER with no error: the raster
// earth renders but GeoJSON sources never parse and `idle` never fires. The
// fix is the vendored worker + shared modules in public/maplibre/ (pinned
// byte-identical by maplibreVendored.test.ts), pointed at BEFORE any Map is
// constructed: the worker pool is a module-global singleton, so a late
// setWorkerUrl is ignored by maps that already spun the pool.
//
// Every module that constructs a Map imports this one for its side effect, so
// the pool is aimed at the vendored copy no matter which map surface loads
// first.
if (typeof window !== 'undefined') {
  maplibregl.setWorkerUrl('/maplibre/maplibre-gl-worker.mjs')
}
