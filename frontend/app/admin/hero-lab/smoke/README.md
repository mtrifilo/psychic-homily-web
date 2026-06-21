# /admin/hero-lab/smoke — spotlight + stage-smoke comparison (PSY-1137)

**Admin-only dev tool** (gated by `AdminGuard` via the `/admin` layout — not a
user-facing surface). Sub-route of the Hero Lab: spotlight-cutoff fixes + the
stage-smoke / god-ray atmosphere mocks. The A–E hero gallery lives at the parent
`/admin/hero-lab`. Delete if the smoke exploration is abandoned. Full rationale +
decisions: `docs/features/hero-wordmark-animation.md`.

## What's here

Spotlight mocks (chosen: #3 `cells`, now live on the homepage):
1. Full-bleed glow layer · 2. Oversized canvas · 3. Cells only ✓

Stage-smoke mocks (exploration; none shipped to the homepage yet):
4. Photoreal video (Mixkit footage + `mix-blend-mode: screen`)
5. Interactive 2D fluid (`webgl-fluid`)
6. Volumetric raymarch fog + god-rays (`VolumetricSmoke.tsx`)

> WebGL mocks (5, 6) render **blank in headless Chromium** (no WebGL2). View in a
> real browser; to screenshot them with agent-browser use `--headed`.

## Regenerating the smoke clips (gitignored)

The `.mp4`s under `public/hero-lab/` are gitignored (heavy binaries). To rebuild:

```bash
# Mixkit free license, no attribution. (asset id 1960 = white smoke on black, 1967 = soft smoke)
curl -o public/hero-lab/smoke-white.mp4 https://assets.mixkit.co/videos/1960/1960-720.mp4
curl -o public/hero-lab/smoke-soft.mp4  https://assets.mixkit.co/videos/1967/1967-720.mp4

# Smooth slow-motion (video has no frame interpolation; slowing playbackRate stutters):
ffmpeg -y -i public/hero-lab/smoke-white.mp4 \
  -vf "setpts=2.2*PTS,minterpolate=fps=30:mi_mode=mci:mc_mode=aobmc" \
  -an -c:v libx264 -crf 23 -pix_fmt yuv420p public/hero-lab/smoke-white-slow.mp4
```

## Deferred

Screen-space radial god-rays — a separate mock to build later (the technique that
yields crisp sunbeam shafts; the volumetric in-scatter in #6 reads as atmospheric
backlit fog, not hard beams).
