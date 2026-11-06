/**
 * Parameters of the Flores Bloom particle engine.
 *
 * Two published versions of the same demo exist, and both can be reproduced:
 * - pmndrs/examples · gpgpu-curl-noise-dof (PRESETS.pmndrs, the DEFAULTS).
 * - Maxime Heckel's r3f.maximeheckel.com/dof-fbo (PRESETS.maxime): cube seeds,
 *   3 octaves, a softer opacity falloff and a wide, faster camera.
 *
 * Names follow pmndrs; Maxime's panel calls `curl` "frequency" and `speed`
 * "speedFactor". `blur` is the point-size factor itself (pmndrs derives it from
 * an aperture: blur = (5.6 - aperture) * 9). Note that `fov` is NOT the camera's
 * field of view (that is `lens`): it draws 1/fov of the particles, 0 = all.
 */
export interface BloomParams {
  /** Distance from the camera that stays sharp. */
  focus: number;
  /** Animation speed multiplier ("speedFactor" in Maxime's panel). */
  speed: number;
  /** Point size per unit of distance from the focus plane: bigger = softer bokeh. */
  blur: number;
  /** Draws 1/fov of the particles; 0 or 1 = all 262,144. */
  fov: number;
  /** Curl noise frequency: the scale of the swirls ("frequency" in Maxime's panel). */
  curl: number;
  /** Extra curl octaves (0–4): 4 in pmndrs, 3 in Maxime's. Each one less saves ~1/5 of the vertex work. */
  detail: number;
  /** Seed shape: a sphere of radius 128 (pmndrs) or a cube of side 4 (Maxime: smoother, larger swirls). */
  seeds: "sphere" | "cube";
  /** Opacity loss per unit of distance from the focus plane: 1.5 in pmndrs, 1 in Maxime's. */
  falloff: number;
  /**
   * 1 = the particles hidden by `fov` are still drawn as 1-pixel dust, as in both
   * references (their gl_PointSize = 0 is clamped to 1 by the GPU). 0 = skip them:
   * up to 1/fov of the work, without the fine dust texture.
   */
  dust: number;

  /** Camera: vertical field of view in degrees (25 in pmndrs, 75 in Maxime's). */
  lens: number;
  /** Camera distance to the centre (6 in pmndrs, 5 in Maxime's). */
  distance: number;
  /** Orbit auto-rotation, in OrbitControls units: 1 = one turn per minute. 0 + shake 0 = a front view. */
  rotate: number;
  /** Hand-held camera shake intensity (drei CameraShake; 0 = off). */
  shake: number;

  /**
   * Screen position of the shape, in half screens: x -1 = left edge, 1 = right
   * edge; y -1 = bottom, 1 = top. An image shift: the perspective is unchanged.
   * On portrait screens x is scaled down (centred below ~3:5) so the shape
   * stays on screen.
   */
  x: number;
  y: number;

  /** Flower mode, 0–1: 0 = the reference cloud, 1 = petals; values between morph. */
  bloom: number;
  /** Number of petals (used when `bloom` > 0). */
  petals: number;
  /** Rings of petals; inner rings are smaller and offset by half a petal. */
  layers: number;
  /** How much of the curl motion is added on top of the petals. */
  turbulence: number;
  /** How much the petals curve towards the camera (depth, so the blur varies). */
  cup: number;
  /** Petal width: 1 = neighbouring petals touch, lower = thinner petals. */
  width: number;
  /** Spin around the viewing axis, in turns per minute. */
  spin: number;
}

const FLOWER = { x: 0, y: 0, bloom: 0, petals: 6, layers: 2, turbulence: 0.35, cup: 0.4, width: 0.85, spin: 0 } as const;

export const PRESETS = {
  /** pmndrs/examples · gpgpu-curl-noise-dof, with its leva defaults. */
  pmndrs: {
    focus: 5.1, speed: 100, blur: (5.6 - 1.8) * 9, fov: 20, curl: 0.25, detail: 4, seeds: "sphere", falloff: 1.5, dust: 1,
    lens: 25, distance: 6, rotate: 0.5, shake: 0.2, ...FLOWER,
  },
  /** r3f.maximeheckel.com/dof-fbo, with its leva defaults. */
  maxime: {
    focus: 5, speed: 50, blur: 30, fov: 60, curl: 0.2, detail: 3, seeds: "cube", falloff: 1, dust: 1,
    lens: 75, distance: 5, rotate: 2, shake: 0, ...FLOWER,
  },
} satisfies Record<string, BloomParams>;

export const DEFAULTS: BloomParams = PRESETS.pmndrs;

/** Slider ranges for /dev/bloom (the references' ranges, widened where both differ). */
export const RANGES = {
  focus: { min: 0, max: 12, step: 0.01 },
  speed: { min: 0.1, max: 100, step: 0.1 },
  blur: { min: 0, max: 50, step: 0.1 },
  fov: { min: 0, max: 200, step: 1 },
  curl: { min: 0, max: 1, step: 0.001 },
  detail: { min: 0, max: 4, step: 1 },
  falloff: { min: 0.25, max: 3, step: 0.05 },
  dust: { min: 0, max: 1, step: 1 },
  lens: { min: 10, max: 100, step: 1 },
  distance: { min: 1, max: 12, step: 0.1 },
  rotate: { min: -4, max: 4, step: 0.1 },
  shake: { min: 0, max: 1, step: 0.01 },
  x: { min: -1, max: 1, step: 0.01 },
  y: { min: -1, max: 1, step: 0.01 },
  bloom: { min: 0, max: 1, step: 0.01 },
  petals: { min: 1, max: 16, step: 1 },
  layers: { min: 1, max: 3, step: 1 },
  turbulence: { min: 0, max: 1, step: 0.01 },
  cup: { min: -1, max: 1, step: 0.01 },
  width: { min: 0.1, max: 1.5, step: 0.01 },
  spin: { min: -6, max: 6, step: 0.1 },
} as const;

/** Choices for the parameters that are not numbers (used by /dev/bloom). */
export const OPTIONS = {
  seeds: ["sphere", "cube"],
} as const;

/** The numeric parameters: the ones that ease and interpolate between scenes. */
export type NumericKey = { [K in keyof BloomParams]: BloomParams[K] extends number ? K : never }[keyof BloomParams];
export const NUMERIC_KEYS = Object.keys(DEFAULTS).filter(
  (k) => typeof DEFAULTS[k as keyof BloomParams] === "number",
) as NumericKey[];

/** Side of the reference's simulation texture: 512 × 512 particles at most. */
export const GRID = 512;
export const MAX_PARTICLES = GRID * GRID;

/** Particles drawn for a given `fov`, as the reference's `step(1 - 1/fov, uv.x)` does. */
export function visibleCount(fov: number): number {
  if (!(fov > 1)) return MAX_PARTICLES;
  return Math.min(MAX_PARTICLES, Math.ceil(MAX_PARTICLES / fov));
}
