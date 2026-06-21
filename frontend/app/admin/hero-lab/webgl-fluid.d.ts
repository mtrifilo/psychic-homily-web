// Minimal ambient types for the untyped `webgl-fluid` package (dev-scratch
// /hero-lab smoke mock). Remove with the mock if the smoke idea is dropped.
declare module 'webgl-fluid' {
  interface WebGLFluidConfig {
    TRIGGER?: 'hover' | 'click'
    IMMEDIATE?: boolean
    AUTO?: boolean
    INTERVAL?: number
    SPLAT_COUNT?: number
    SIM_RESOLUTION?: number
    DYE_RESOLUTION?: number
    CAPTURE_RESOLUTION?: number
    DENSITY_DISSIPATION?: number
    VELOCITY_DISSIPATION?: number
    PRESSURE?: number
    PRESSURE_ITERATIONS?: number
    CURL?: number
    SPLAT_RADIUS?: number
    SPLAT_FORCE?: number
    SPLAT_COLOR?: { r: number; g: number; b: number }
    SHADING?: boolean
    COLORFUL?: boolean
    COLOR_UPDATE_SPEED?: number
    PAUSED?: boolean
    BACK_COLOR?: { r: number; g: number; b: number }
    TRANSPARENT?: boolean
    BLOOM?: boolean
    SUNRAYS?: boolean
    [key: string]: unknown
  }
  const WebGLFluid: (canvas: HTMLCanvasElement, config?: WebGLFluidConfig) => void
  export default WebGLFluid
}
