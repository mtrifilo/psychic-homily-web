'use client'

/**
 * Volumetric raymarched smoke (dev-scratch /hero-lab mock).
 *
 * A fullscreen-quad fragment shader raymarches a 3D fbm noise field as DENSITY
 * (Beer–Lambert accumulation + a cheap light-march for self-shadowing — the
 * thing that makes it read as a lit volume rather than flat fog). The volume
 * drifts upward on uTime (a "smoke machine" rising). The cursor isn't a real
 * fluid solver — it injects a decaying local force that pushes the noise domain,
 * so the haze "swirls" where you move. Looks 3D/volumetric; the motion is faked.
 *
 * Self-contained WebGL2 (no three.js). Composites over a dark bg with premult
 * alpha; place behind the wordmark. Plain JS WebGL — no React-render churn.
 */

import { useEffect, useRef } from 'react'

const VERT = `#version 300 es
precision highp float;
const vec2 P[3] = vec2[3](vec2(-1.0,-1.0), vec2(3.0,-1.0), vec2(-1.0,3.0));
void main(){ gl_Position = vec4(P[gl_VertexID], 0.0, 1.0); }
`

const FRAG = `#version 300 es
precision highp float;
out vec4 outColor;
uniform vec2 uResolution;
uniform float uTime;
uniform vec2 uMouse;      // 0..1, y up
uniform float uMouseStr;  // decaying disturbance strength
uniform vec3 uWarm;

float hash(vec3 p){ p = fract(p*0.3183099 + 0.1); p *= 17.0; return fract(p.x*p.y*p.z*(p.x+p.y+p.z)); }
float noise(vec3 x){
  vec3 i = floor(x), f = fract(x); f = f*f*(3.0-2.0*f);
  return mix(mix(mix(hash(i+vec3(0,0,0)),hash(i+vec3(1,0,0)),f.x),
                 mix(hash(i+vec3(0,1,0)),hash(i+vec3(1,1,0)),f.x),f.y),
             mix(mix(hash(i+vec3(0,0,1)),hash(i+vec3(1,0,1)),f.x),
                 mix(hash(i+vec3(0,1,1)),hash(i+vec3(1,1,1)),f.x),f.y), f.z);
}
float fbm(vec3 p){ float v=0.0,a=0.5; for(int i=0;i<5;i++){ v+=a*noise(p); p*=2.02; a*=0.5; } return v; }

float density(vec3 p, vec2 m){
  // slow roll drift
  vec3 q = p + vec3(uTime*0.045, uTime*0.085, uTime*0.02);
  // domain warp — the key to billowing, cauliflower fog that FILLS the space and
  // rolls, instead of flat haze or columns. (warp the lookup by cheap noise.)
  vec3 w = vec3(noise(q*0.7 + 2.0), noise(q*0.7 + 7.3), noise(q*0.7 + 4.1)) - 0.5;
  q += w * 1.5;
  // cursor disturbance (decays); pushes the fog away from the pointer
  float md = length(p.xy - m);
  q.xy += normalize(p.xy - m + 1e-3) * uMouseStr * exp(-md*2.0) * 0.4;
  float d = fbm(q*1.05) - 0.5; // higher threshold → clearer gaps for light to beam through
  return clamp(d, 0.0, 1.0);
}

// Henyey–Greenstein phase — forward scattering pools light toward the source,
// which is what carves visible beams/shafts out of the fog.
float hg(float c, float g){
  float g2 = g*g;
  return (1.0 - g2) / (12.566 * pow(max(1.0 + g2 - 2.0*g*c, 1e-3), 1.5));
}

void main(){
  vec2 uv = (gl_FragCoord.xy - 0.5*uResolution) / uResolution.y;
  float aspect = uResolution.x / uResolution.y;
  vec3 ro = vec3(0.0, 0.0, -3.2);
  vec3 rd = normalize(vec3(uv, 1.5));
  vec2 m = (uMouse - 0.5) * vec2(aspect, 1.0);

  // Directional stage light from above-front → parallel god-ray shafts. Beams
  // appear where the path toward the light is clear; billows cast the shadow
  // streaks between them. (Crepuscular rays are parallel, hence directional.)
  vec3 L = normalize(vec3(0.12, 1.0, 0.35)); // direction TOWARD the light
  vec3 lightCol = vec3(3.3, 3.22, 3.12);     // bright, near-neutral white
  float ph = hg(dot(rd, L), 0.72);           // tighter forward lobe → crisper beams

  vec3 col = vec3(0.0);
  float trans = 1.0;
  float t = 1.4;
  for(int i=0;i<48;i++){
    vec3 pos = ro + rd*t;
    float dens = density(pos, m);
    if(dens > 0.01){
      // shadow-march toward the light: occluded samples stay dark → carves shafts
      float sh = 0.0; vec3 sp = pos;
      for(int j=0;j<7;j++){ sp += L*0.3; sh += density(sp, m); }
      float vis = exp(-sh * 2.4);
      vec3 body  = uWarm * (0.2 + 0.3*vis);     // dim fog so the beams stand out
      vec3 shaft = lightCol * pow(vis, 1.4) * ph; // bright, sharper parallel beams
      float a = dens * 0.6;
      col += trans * a * (body + shaft);   // premultiplied
      trans *= 1.0 - a;
      if(trans < 0.02) break;
    }
    t += 0.11;
  }
  float alpha = 1.0 - trans;
  col = min(col, vec3(1.0)); // keep fog brightness; bright shafts clip to white

  // Fog settles heavier toward the bottom, thins upward (pools low like a stage).
  float vgrad = mix(1.0, 0.6, smoothstep(-0.55, 0.85, uv.y));
  alpha *= vgrad;
  col   *= vgrad;

  outColor = vec4(col, alpha);
}
`

function compile(gl: WebGL2RenderingContext, type: number, src: string) {
  const sh = gl.createShader(type)!
  gl.shaderSource(sh, src)
  gl.compileShader(sh)
  if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
    console.error('smoke shader error:', gl.getShaderInfoLog(sh))
  }
  return sh
}

export function VolumetricSmoke({ className }: { className?: string }) {
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

    const uRes = gl.getUniformLocation(prog, 'uResolution')
    const uTime = gl.getUniformLocation(prog, 'uTime')
    const uMouse = gl.getUniformLocation(prog, 'uMouse')
    const uMouseStr = gl.getUniformLocation(prog, 'uMouseStr')
    const uWarm = gl.getUniformLocation(prog, 'uWarm')
    gl.uniform3f(uWarm, 0.74, 0.72, 0.7) // near-neutral gray, only a hair warm

    // Half-resolution render for perf; CSS upscales it.
    const scale = 0.5
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const mouse = { x: 0.5, y: 0.5, str: 0, lx: 0.5, ly: 0.5 }
    const resize = () => {
      const r = canvas.getBoundingClientRect()
      canvas.width = Math.max(1, Math.round(r.width * dpr * scale))
      canvas.height = Math.max(1, Math.round(r.height * dpr * scale))
      gl.viewport(0, 0, canvas.width, canvas.height)
    }
    resize()
    const ro = new ResizeObserver(resize)
    ro.observe(canvas)

    const onMove = (e: PointerEvent) => {
      const r = canvas.getBoundingClientRect()
      const nx = (e.clientX - r.left) / r.width
      const ny = 1 - (e.clientY - r.top) / r.height
      mouse.str = Math.min(1.4, mouse.str + Math.hypot(nx - mouse.x, ny - mouse.y) * 6)
      mouse.x = nx
      mouse.y = ny
    }
    canvas.addEventListener('pointermove', onMove)

    let raf = 0
    const start = performance.now()
    const frame = (now: number) => {
      mouse.str *= 0.93 // decay the disturbance
      gl.uniform2f(uRes, canvas.width, canvas.height)
      gl.uniform1f(uTime, (now - start) / 1000)
      gl.uniform2f(uMouse, mouse.x, mouse.y)
      gl.uniform1f(uMouseStr, mouse.str)
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
