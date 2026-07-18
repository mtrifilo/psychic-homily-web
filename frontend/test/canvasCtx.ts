/**
 * Minimal canvas 2D context stub for graph node-paint tests, shared by the
 * ForceGraphView and ArtistGraphVisualization suites so it can't drift per
 * file. Records every globalAlpha assignment so the hover/pin focus dim can
 * be asserted — nodeCanvasObject resets globalAlpha to 1 at the end of each
 * node, so the live value can't be read after the call. Covers the superset
 * of ctx members both surfaces' node paint paths touch (rings, dashes, the
 * suggested-direction badge); add here, once, when a paint pass grows a new
 * canvas call.
 */
export interface FakeCanvasCtx {
  /** Every value assigned to globalAlpha, in order. */
  alphas: number[]
  globalAlpha: number
  fillStyle: string
  strokeStyle: string
  lineWidth: number
  shadowColor: string
  shadowBlur: number
  beginPath(): void
  closePath(): void
  arc(): void
  fill(): void
  stroke(): void
  setLineDash(): void
  save(): void
  restore(): void
  moveTo(): void
  lineTo(): void
}

export function makeFakeCtx(): FakeCanvasCtx {
  const alphas: number[] = []
  let alpha = 1
  return {
    get globalAlpha() {
      return alpha
    },
    set globalAlpha(v: number) {
      alpha = v
      alphas.push(v)
    },
    beginPath() {},
    closePath() {},
    arc() {},
    fill() {},
    stroke() {},
    setLineDash() {},
    save() {},
    restore() {},
    moveTo() {},
    lineTo() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    shadowColor: '',
    shadowBlur: 0,
    alphas,
  }
}
