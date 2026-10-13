#!/usr/bin/env node
// Renders the raster app icons from app/favicon.svg:
//   app/icon.png               192x192, the rel=icon PNG and the manifest's 192
//   public/icons/icon-512.png  512x512, the manifest's 512 (any and maskable)
//
// Each is flattened onto black and written as a palette PNG. The script exits
// non-zero, writing nothing, if any lit pixel of the 512 falls outside the
// maskable safe zone (the centred circle of radius 40%), because the manifest
// lists that file as maskable. app/apple-icon.png is separate artwork and is
// not rendered here.
//
// sharp resolves through Next's own dependency on it; it is not a direct
// dependency of this package.
//
// Usage (from frontend/):
//   node scripts/app-icons.mjs
import sharp from 'sharp'

const SOURCE = 'app/favicon.svg'
const OUTPUTS = [
  { size: 192, path: 'app/icon.png' },
  { size: 512, path: 'public/icons/icon-512.png' },
]
const SAFE_ZONE_RADIUS = 0.4
// A channel above this counts as artwork rather than background.
const LIT_THRESHOLD = 40

function render(size) {
  return sharp(SOURCE).resize(size, size).flatten({ background: '#000000' })
}

async function farthestLitPixelRatio(size) {
  const { data, info } = await render(size).raw().toBuffer({ resolveWithObject: true })
  const centre = size / 2
  let farthest = 0
  for (let y = 0; y < info.height; y++) {
    for (let x = 0; x < info.width; x++) {
      const i = (y * info.width + x) * info.channels
      if (Math.max(data[i], data[i + 1], data[i + 2]) > LIT_THRESHOLD) {
        farthest = Math.max(farthest, Math.hypot(x + 0.5 - centre, y + 0.5 - centre))
      }
    }
  }
  return farthest / size
}

const ratio = await farthestLitPixelRatio(512)
if (ratio > SAFE_ZONE_RADIUS) {
  console.error(
    `artwork reaches ${ratio.toFixed(3)} of the width from centre; the maskable safe zone is ${SAFE_ZONE_RADIUS}`
  )
  process.exit(1)
}
for (const { size, path } of OUTPUTS) {
  await render(size)
    .png({ palette: true, quality: 100, dither: 0, colours: 256, compressionLevel: 9, effort: 10 })
    .toFile(path)
  console.log(`wrote ${path} (${size}x${size})`)
}
console.log(`farthest lit pixel: ${ratio.toFixed(3)} of the width (safe zone ${SAFE_ZONE_RADIUS})`)
