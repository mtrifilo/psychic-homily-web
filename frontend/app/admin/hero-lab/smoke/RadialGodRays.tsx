'use client'

/**
 * Screen-space radial god-rays (dev-scratch /admin/hero-lab/smoke mock 7, PSY-1161).
 *
 * The crisp-beam counterpart to the volumetric fog (mock 6). Where mock 6's
 * directional in-scatter reads as soft backlit haze, this is the post-process
 * "light scattering" technique (GPU Gems 3, ch. 13): radially blur a bright
 * EMITTER outward from a light's screen position, accumulating with exponential
 * decay. The dark gaps between the wordmark's light-cells become the striping
 * that reads as hard sunbeam shafts.
 *
 * Emitter = the actual PSYCHIC HOMILY dots (sampled from the production font,
 * drawn bright on black, uploaded once) + a soft backlight disc added in-shader
 * at the light position. So the beams stream from the real letters. The light
 * follows the cursor; the crisp dots are composited back on top so the wordmark
 * stays legible.
 *
 * Self-contained WebGL2 (no three.js). agent-browser headless has no WebGL2 —
 * verify in a real browser / `--headed`.
 */

import { useEffect, useRef } from 'react'
import { readWordmarkColors, resolveFontFamily, sampleWordmark } from '@/features/home/components/scrying-grid/sampleWordmark'

const LINES = ['PSYCHIC', 'HOMILY'] as const

const VERT = `#version 300 es
precision highp float;
const vec2 P[3] = vec2[3](vec2(-1.0,-1.0), vec2(3.0,-1.0), vec2(-1.0,3.0));
out vec2 vUv;
void main(){
  vec2 p = P[gl_VertexID];
  vUv = 0.5 * (p + 1.0);
  gl_Position = vec4(p, 0.0, 1.0);
}
`

// Radial light-scattering post-process (GPU Gems 3, ch.38/ch.13 lineage).
const FRAG = `#version 300 es
precision highp float;
in vec2 vUv;
out vec4 outColor;
uniform sampler2D uDots;   // wordmark dots, bright on black (premade)
uniform vec2 uLight;       // light position, 0..1 (y up)
uniform vec3 uBeam;        // beam tint
uniform float uExposure;

const int SAMPLES = 64;
const float DENSITY = 0.85;
const float DECAY = 0.962;
const float WEIGHT = 0.42;

// Emitter at a uv = the dots there + a soft backlight disc at the light.
vec3 emitter(vec2 uv){
  vec3 dots = texture(uDots, uv).rgb;
  float d = distance(vec2(uv.x, 1.0 - uv.y), vec2(uLight.x, 1.0 - uLight.y));
  float disc = smoothstep(0.34, 0.0, d);
  return dots + uBeam * disc * 0.9;
}

void main(){
  vec2 uv = vUv;
  vec2 step = (uv - uLight) * (DENSITY / float(SAMPLES));
  vec2 coord = uv;
  float decay = 1.0;
  vec3 sum = vec3(0.0);
  for(int i=0;i<SAMPLES;i++){
    coord -= step;
    sum += emitter(coord) * decay * WEIGHT;
    decay *= DECAY;
  }
  // Beams only — a crisp ScryingGridWordmark is overlaid on top for the letters.
  vec3 col = sum * uExposure;
  col = col / (1.0 + col);                        // Reinhard — beams bloom, no clip
  outColor = vec4(col, max(max(col.r, col.g), col.b));
}
`

function compile(gl: WebGL2RenderingContext, type: number, src: string) {
  const sh = gl.createShader(type)!
  gl.shaderSource(sh, src)
  gl.compileShader(sh)
  if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
    console.error('god-ray shader:', gl.getShaderInfoLog(sh))
  }
  return sh
}

export function RadialGodRays({ className }: { className?: string }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    const gl = canvas.getContext('webgl2', { premultipliedAlpha: true, alpha: true })
    if (!gl) return

    const prog = gl.createProgram()!
    gl.attachShader(prog, compile(gl, gl.VERTEX_SHADER, VERT))
    gl.attachShader(prog, compile(gl, gl.FRAGMENT_SHADER, FRAG))
    gl.linkProgram(prog)
    gl.useProgram(prog)
    const vao = gl.createVertexArray()
    gl.bindVertexArray(vao)

    const uLight = gl.getUniformLocation(prog, 'uLight')
    const uBeam = gl.getUniformLocation(prog, 'uBeam')
    const uExposure = gl.getUniformLocation(prog, 'uExposure')
    gl.uniform1f(uExposure, 1.15)

    // Emitter texture: the wordmark dots, warm-white on black.
    const tex = gl.createTexture()
    gl.bindTexture(gl.TEXTURE_2D, tex)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    gl.uniform1i(gl.getUniformLocation(prog, 'uDots'), 0)

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const scale = 0.6 // god-ray loop is 64 taps/px — render at 0.6×, CSS upscales
    // Default off to the right so beams rake across the wordmark (the dramatic
    // look). The cursor sweeps the light from here.
    const light = { x: 0.8, y: 0.5, tx: 0.8, ty: 0.5 }

    const buildEmitter = () => {
      const rect = canvas.getBoundingClientRect()
      const w = Math.max(1, Math.round(rect.width * dpr * scale))
      const h = Math.max(1, Math.round(rect.height * dpr * scale))
      canvas.width = w
      canvas.height = h
      gl.viewport(0, 0, w, h)
      // warm-white beams from the live primary token
      const { primary } = readWordmarkColors()
      gl.uniform3f(uBeam, primary[0] / 255, primary[1] / 255, primary[2] / 255)

      const off = document.createElement('canvas')
      off.width = w
      off.height = h
      const ctx = off.getContext('2d')
      if (!ctx) return
      ctx.fillStyle = '#000'
      ctx.fillRect(0, 0, w, h)
      const pts = sampleWordmark(w, h, {
        lines: LINES,
        fontFamily: resolveFontFamily(),
        gapDev: Math.round(5 * dpr * scale),
      })
      const r = Math.max(1, 1.5 * dpr * scale)
      ctx.fillStyle = 'rgb(255,244,224)'
      for (const p of pts) {
        ctx.beginPath()
        ctx.arc(p.x, p.y, r, 0, Math.PI * 2)
        ctx.fill()
      }
      gl.bindTexture(gl.TEXTURE_2D, tex)
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, off)
    }
    buildEmitter()
    const ro = new ResizeObserver(buildEmitter)
    ro.observe(canvas)

    const onMove = (e: PointerEvent) => {
      const rect = canvas.getBoundingClientRect()
      light.tx = (e.clientX - rect.left) / rect.width
      light.ty = 1 - (e.clientY - rect.top) / rect.height
    }
    canvas.addEventListener('pointermove', onMove)

    let raf = 0
    const frame = () => {
      // ease the light toward the cursor for a smooth sweep
      light.x += (light.tx - light.x) * 0.08
      light.y += (light.ty - light.y) * 0.08
      gl.uniform2f(uLight, light.x, light.y)
      gl.clearColor(0, 0, 0, 0)
      gl.clear(gl.COLOR_BUFFER_BIT)
      gl.drawArrays(gl.TRIANGLES, 0, 3)
      raf = requestAnimationFrame(frame)
    }
    raf = requestAnimationFrame(frame)

    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
      canvas.removeEventListener('pointermove', onMove)
    }
  }, [])

  return <canvas ref={ref} aria-hidden className={className} />
}
