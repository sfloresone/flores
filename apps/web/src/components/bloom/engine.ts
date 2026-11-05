/**
 * Flores Bloom: a dependency-free WebGL2 particle engine.
 *
 * Port of pmndrs/examples · gpgpu-curl-noise-dof (MIT) without three.js, an FBO,
 * float textures or vertex buffers: each particle derives its seed from
 * gl_VertexID and runs the stateless simulation in the vertex shader. Nothing is
 * allocated per frame. See README.md.
 */
import { perspective, viewMatrix } from "./camera";
import FRAGMENT from "./fragment.glsl";
import { DEFAULTS, MAX_PARTICLES, visibleCount, type BloomParams } from "./params";
import VERTEX from "./vertex.glsl";

/** Development-only measurements (always zero in production builds). */
export interface BloomStats {
  /** Frames per second, averaged over the last half second. */
  fps: number;
  /** Mean CPU time spent preparing and submitting a frame (ms). */
  cpuMs: number;
  /** Mean GPU time per frame (ms), when EXT_disjoint_timer_query_webgl2 exists. */
  gpuMs: number | null;
  /** Particles drawn in the last frame. */
  particles: number;
  /** Current render pixel ratio (adaptive quality may lower it). */
  dpr: number;
  /** Curl octaves in use (adaptive quality may lower them below `detail`). */
  detail: number;
}

export interface BloomOptions {
  /** Pause when the canvas is off-screen or the tab is hidden (default true). */
  autoPause?: boolean;
  /** Render a single still frame (default: follows prefers-reduced-motion). */
  still?: boolean;
  /** Cap for devicePixelRatio. 2 matches react-three-fiber's default. */
  maxDpr?: number;
  /** Lower the render resolution on slow devices (default true). */
  adaptive?: boolean;
  /** Called if the shaders fail to build (asynchronously): show a fallback. */
  onError?: (error: Error) => void;
}

export interface Bloom {
  readonly params: BloomParams;
  readonly stats: BloomStats;
  set(params: Partial<BloomParams>): void;
  start(): void;
  stop(): void;
  destroy(): void;
}

const NEAR = 0.1; // react-three-fiber's default camera
const FAR = 1000;
const LERP = 0.1; // per-frame easing of the uniforms, as in the reference
/** Point sprites larger than this (in DPR-2 pixels) are capped: same on every device. */
const MAX_POINT = 96;
/** Adaptive quality: every WARMUP frames, a median frame time above SLOW_MS lowers the DPR, then `detail`. */
const WARMUP = 60;
const SLOW_MS = 18;

const UNIFORMS = [
  "uProjection", "uView", "uTime", "uCurlFreq", "uFocus", "uBlur", "uPixelRatio", "uMaxPoint",
  "uDetail", "uVisible", "uCubeSeeds", "uFalloff", "uPetals", "uLayers", "uTurbulence", "uCup", "uWidth",
] as const;
type Locations = Record<(typeof UNIFORMS)[number], WebGLUniformLocation | null>;

type TimerExt = { TIME_ELAPSED_EXT: number; GPU_DISJOINT_EXT: number };

function shader(gl: WebGL2RenderingContext, type: number, src: string): WebGLShader {
  const sh = gl.createShader(type)!;
  gl.shaderSource(sh, src);
  gl.compileShader(sh); // status is read later: see ready()
  return sh;
}

/**
 * Create the engine on a canvas. Returns null when WebGL2 is unavailable, so the
 * caller can show a static fallback.
 */
export function createBloom(
  canvas: HTMLCanvasElement,
  initial: Partial<BloomParams> = {},
  options: BloomOptions = {},
): Bloom | null {
  const autoPause = options.autoPause ?? true;
  const adaptive = options.adaptive ?? true;
  const still = options.still ?? matchMedia("(prefers-reduced-motion: reduce)").matches;

  const params: BloomParams = { ...DEFAULTS, ...initial };
  // Eased uniforms: they start at the given values and then follow set() smoothly.
  let eFocus = params.focus, eFov = params.fov, eBlur = params.blur, eCurl = params.curl;

  const projection = new Float32Array(16);
  const view = new Float32Array(16);

  let gl: WebGL2RenderingContext | null = null;
  let program: WebGLProgram | null = null;
  let shaders: WebGLShader[] = []; // until linked
  let parallel: { COMPLETION_STATUS_KHR: number } | null = null;
  let loc = {} as Locations;
  let raf = 0;
  let running = false; // the caller wants it running
  let visible = true;
  let elapsed = 0;
  let spinAngle = 0;
  let last = 0;

  // Size, read only by the ResizeObserver: no layout reads in the frame loop.
  let cssW = canvas.clientWidth, cssH = canvas.clientHeight;
  let dpr = Math.min(devicePixelRatio || 1, options.maxDpr ?? 2);
  let sizeDirty = true;
  let paramsDirty = true; // the uniforms that only change through set()

  // Adaptive quality: a ring of recent frame times.
  const frameTimes = new Float32Array(WARMUP);
  const sorted = new Float32Array(WARMUP); // scratch for the median: no allocation
  let frameCount = 0;
  let maxDetail = 4; // adaptive quality may lower the curl octaves

  const stats: BloomStats = { fps: 0, cpuMs: 0, gpuMs: null, particles: 0, dpr, detail: 0 };
  // Development-only state; `import.meta.env.DEV` removes it from production builds.
  let timer: TimerExt | null = null, query: WebGLQuery | null = null;
  let statFrames = 0, statCpu = 0, statStart = 0, gpuSum = 0, gpuN = 0;

  function init(): boolean {
    gl = canvas.getContext("webgl2", {
      alpha: false, // opaque black: cheaper to composite, identical on a black page
      antialias: false, // MSAA does nothing for round sprites cut by discard
      depth: false,
      stencil: false,
      powerPreference: "low-power", // decorative: prefer the integrated GPU
    });
    if (!gl) return false;
    // Compile and link without asking for the result: with KHR_parallel_shader_compile
    // the driver builds the program off the main thread, so page load never waits for it.
    parallel = gl.getExtension("KHR_parallel_shader_compile");
    shaders = [shader(gl, gl.VERTEX_SHADER, VERTEX), shader(gl, gl.FRAGMENT_SHADER, FRAGMENT)];
    program = gl.createProgram()!;
    for (const sh of shaders) gl.attachShader(program, sh);
    gl.linkProgram(program);

    // Same blending as three.js NormalBlending on a non-premultiplied material.
    gl.enable(gl.BLEND);
    gl.blendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.clearColor(0, 0, 0, 1);

    if (import.meta.env.DEV) {
      timer = gl.getExtension("EXT_disjoint_timer_query_webgl2") as TimerExt | null;
      query = null;
    }
    sizeDirty = paramsDirty = true;
    return true;
  }

  /** True once the program is linked and set up; false while it is still building. */
  function ready(g: WebGL2RenderingContext): boolean {
    if (!shaders.length) return true;
    if (parallel && !g.getProgramParameter(program!, parallel.COMPLETION_STATUS_KHR)) return false;
    if (!g.getProgramParameter(program!, g.LINK_STATUS)) {
      const logs = shaders.map((sh) => g.getShaderInfoLog(sh)).join("\n");
      fail(new Error(`flores-bloom: shader build failed: ${g.getProgramInfoLog(program!)}\n${logs}`));
      return false;
    }
    for (const sh of shaders) {
      g.detachShader(program!, sh);
      g.deleteShader(sh);
    }
    shaders = [];
    loc = {} as Locations;
    for (const u of UNIFORMS) loc[u] = g.getUniformLocation(program!, u);
    g.useProgram(program);
    g.uniform1f(loc.uMaxPoint, MAX_POINT);
    return true;
  }

  function fail(error: Error) {
    api.destroy();
    if (options.onError) options.onError(error);
    else console.error(error);
  }

  function applySize(g: WebGL2RenderingContext) {
    const w = Math.max(1, Math.round(cssW * dpr));
    const h = Math.max(1, Math.round(cssH * dpr));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
    }
    g.viewport(0, 0, w, h);
    g.uniformMatrix4fv(loc.uProjection, false, perspective(projection, params.lens, w / h, NEAR, FAR));
    g.uniform1f(loc.uPixelRatio, dpr);
    stats.dpr = dpr;
    sizeDirty = false;
  }

  function applyParams(g: WebGL2RenderingContext) {
    stats.detail = Math.min(Math.round(params.detail), maxDetail);
    g.uniform1i(loc.uDetail, stats.detail);
    g.uniform1i(loc.uCubeSeeds, params.seeds === "cube" ? 1 : 0);
    g.uniform1f(loc.uFalloff, params.falloff);
    g.uniform1f(loc.uPetals, Math.round(params.petals));
    g.uniform1f(loc.uLayers, Math.round(params.layers));
    g.uniform1f(loc.uTurbulence, params.turbulence);
    g.uniform1f(loc.uCup, params.cup);
    g.uniform1f(loc.uWidth, params.width);
    paramsDirty = false;
  }

  /** Draws one frame; false when the program is not ready yet. */
  function draw(ease: boolean): boolean {
    const g = gl;
    if (!g || !ready(g)) return false;
    const t0 = import.meta.env.DEV ? performance.now() : 0;
    const k = ease ? LERP : 1;
    eFocus += (params.focus - eFocus) * k;
    eFov += (params.fov - eFov) * k;
    eBlur += (params.blur - eBlur) * k;
    eCurl += (params.curl - eCurl) * k;
    const visible = visibleCount(eFov);
    const count = params.dust ? MAX_PARTICLES : visible;

    if (sizeDirty) applySize(g);
    if (paramsDirty) applyParams(g);

    // GPU time: one query in flight at a time; frames while it is pending are not timed.
    let timing = false;
    if (import.meta.env.DEV && timer) {
      if (query && g.getQueryParameter(query, g.QUERY_RESULT_AVAILABLE)) {
        if (!g.getParameter(timer.GPU_DISJOINT_EXT)) {
          gpuSum += g.getQueryParameter(query, g.QUERY_RESULT) / 1e6;
          gpuN++;
        }
        g.deleteQuery(query);
        query = null;
      }
      if (!query && (query = g.createQuery())) {
        g.beginQuery(timer.TIME_ELAPSED_EXT, query);
        timing = true;
      }
    }

    g.clear(g.COLOR_BUFFER_BIT);
    g.uniformMatrix4fv(loc.uView, false, viewMatrix(view, elapsed, params.view, spinAngle, params));
    g.uniform1f(loc.uTime, elapsed * params.speed);
    g.uniform1f(loc.uCurlFreq, eCurl);
    g.uniform1f(loc.uFocus, eFocus);
    g.uniform1f(loc.uBlur, eBlur);
    g.uniform1i(loc.uVisible, visible);
    g.drawArrays(g.POINTS, 0, count); // no buffers: the shader uses gl_VertexID

    if (import.meta.env.DEV) {
      if (timing && timer) g.endQuery(timer.TIME_ELAPSED_EXT);
      stats.particles = count;
      statCpu += performance.now() - t0;
      statFrames++;
    }
    return true;
  }

  /** Median of the last WARMUP frame times; lowers the DPR one step when slow. */
  function adapt(dtMs: number) {
    if (dpr <= 1 && maxDetail <= 1) return; // nothing left to lower
    frameTimes[frameCount++ % WARMUP] = dtMs;
    if (frameCount < WARMUP || frameCount % WARMUP) return;
    sorted.set(frameTimes);
    if (sorted.sort()[WARMUP >> 1]! <= SLOW_MS) return; // median frame time
    if (dpr > 1) {
      // First the resolution: point sizes do not depend on it, so the look stays.
      dpr = dpr > 1.5 ? 1.5 : 1;
      sizeDirty = true;
    } else {
      // Then the curl octaves (vertex-bound cases, e.g. all 262,144 particles):
      // the swirls lose some fine detail, the shape stays.
      maxDetail = Math.min(maxDetail, Math.round(params.detail)) - 1;
      paramsDirty = true;
    }
  }

  function frame(now: number) {
    raf = 0;
    if (!gl) return;
    const dtMs = last ? now - last : 0;
    last = now;
    const dt = Math.min(dtMs / 1000, 0.1);
    elapsed += dt;
    spinAngle += ((params.spin * Math.PI * 2) / 60) * dt;
    draw(true);
    if (adaptive && dtMs) adapt(dtMs);

    if (import.meta.env.DEV) {
      if (!statStart) statStart = now;
      const span = now - statStart;
      if (span >= 500) {
        stats.fps = (statFrames * 1000) / span;
        stats.cpuMs = statCpu / statFrames;
        stats.gpuMs = gpuN ? gpuSum / gpuN : timer ? stats.gpuMs : null;
        statFrames = statCpu = gpuSum = gpuN = 0;
        statStart = now;
      }
    }
    schedule();
  }

  function schedule() {
    if (!still && running && visible && gl && !raf) raf = requestAnimationFrame(frame);
  }

  function renderStill() {
    elapsed = 4; // a representative moment, not the very first frame
    if (!draw(false) && gl && shaders.length) requestAnimationFrame(renderStill); // still building
  }

  function refresh() {
    if (still) renderStill();
    else schedule();
  }

  function halt() {
    cancelAnimationFrame(raf);
    raf = 0;
    last = 0; // the next frame starts with dt = 0
  }

  if (!init()) return null;

  const ro = new ResizeObserver(([entry]) => {
    if (!entry) return;
    cssW = entry.contentRect.width;
    cssH = entry.contentRect.height;
    sizeDirty = true;
    if (still) renderStill();
  });
  ro.observe(canvas);

  const setVisible = (v: boolean) => {
    visible = v;
    if (v) schedule();
    else halt();
  };
  const onVisibility = () => setVisible(!document.hidden);
  const io = autoPause
    ? new IntersectionObserver(([e]) => setVisible(!!e?.isIntersecting && !document.hidden))
    : null;
  if (io) {
    io.observe(canvas);
    document.addEventListener("visibilitychange", onVisibility);
  }

  const onLost = (e: Event) => {
    e.preventDefault();
    halt();
    gl = null;
  };
  const onRestored = () => {
    if (init()) refresh();
  };
  canvas.addEventListener("webglcontextlost", onLost);
  canvas.addEventListener("webglcontextrestored", onRestored);

  const api: Bloom = {
    params,
    stats,
    set(next) {
      Object.assign(params, next);
      paramsDirty = sizeDirty = true; // sizeDirty: the projection depends on `lens`
      if (still) renderStill();
    },
    start() {
      running = true;
      refresh();
    },
    stop() {
      running = false;
      halt();
    },
    destroy() {
      api.stop();
      ro.disconnect();
      io?.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
      canvas.removeEventListener("webglcontextlost", onLost);
      canvas.removeEventListener("webglcontextrestored", onRestored);
      if (gl) {
        for (const sh of shaders) gl.deleteShader(sh);
        gl.deleteProgram(program);
      }
      shaders = [];
      gl = program = null;
    },
  };
  return api;
}
