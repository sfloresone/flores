# Flores Bloom

> 🇪🇸 [Español](README.es.md)

The particle "flower" of the flores website: a dependency-free WebGL2 engine.

## Where it comes from

It is a port of the *Depth Of Field & Frame Buffer Object* demo
([r3f.maximeheckel.com/dof-fbo](https://r3f.maximeheckel.com/dof-fbo)), whose
source is [`pmndrs/examples` · `gpgpu-curl-noise-dof`](https://github.com/pmndrs/examples/tree/main/examples/gpgpu-curl-noise-dof)
(MIT). The noise is by Ashima Arts / Stefan Gustavson (MIT) and the curl noise by
[tiagosr/glsl-curl-noise2](https://github.com/tiagosr/glsl-curl-noise2) (MIT).
Full notices: [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Two references

The same demo was published twice, with differences that change the look a lot.
Both are reproduced with parameters, as presets in `params.ts`:

| | `pmndrs` (pmndrs/examples) | `maxime` (r3f.maximeheckel.com/dof-fbo) |
|---|---|---|
| Seeds | sphere of radius 128 | cube of side 4: smoother, larger swirls |
| Curl octaves (`detail`) | 4 | 3 |
| Opacity loss (`falloff`) | 1.5 | 1 |
| Camera (`lens`, `distance`, `rotate`, `shake`) | 25°, 6, 0.5, 0.2 | 75°, 5, 2, 0 |
| Names in its panel | `curl`, `speed`, `aperture` | `frequency`, `speedFactor`, `blur` |

`blur` here is the value used directly (Maxime's); pmndrs's aperture converts as
`blur = (5.6 - aperture) * 9`. In both, the particles hidden by `fov` are still
drawn as 1-pixel dust (`dust: 1`), because the GPU clamps their size 0 to 1.

The landing's story is built on `maxime` with frequency 0.15, speedFactor 34.5,
fov 0, blur 0.9 and focus 6.90: all 262,144 particles, small and sharp (see
[Stories](#stories)).

## How it works, in four ideas

1. **Particles start on a sphere.** Each one has a fixed "seed" point.
2. **Curl noise moves them.** Curl noise is a smooth field that looks like smoke or
   fluid. The position of each particle is a function of its seed and the time.
3. **Fake depth of field.** The further a particle is from the focus distance, the
   bigger and more transparent it is drawn. There is no post-processing.
4. **Flower mode** (`bloom`, 0–1). The petals are built geometrically (each particle
   picks a petal, a place across it and a place from base to tip) and the curl
   motion of the original is added on top as turbulence. Values between 0 and 1
   morph the cloud into the flower.

## Files

| File | What it is |
|---|---|
| `FloresBloom.astro` | The component: a canvas, the fallback and a few lines that start the engine. |
| `engine.ts` | WebGL setup, frame loop, pausing, adaptive quality. |
| `vertex.glsl`, `fragment.glsl` | The shaders. Minified at build time (see `astro.config.ts`). |
| `camera.ts` | The original's camera, without allocations. |
| `params.ts` | Parameters, defaults and ranges. |
| `scenes.ts` | Stories: ordered scenes (full parameter sets) for scroll-driven pages. |
| `story.ts` | The scroll driver: maps the scroll position to an interpolated scene. |

## Why it is light

**Less work than the original.** The original simulates 512 × 512 = 262,144
particles every frame in an off-screen texture (FBO), but draws only `1 / fov` of
them (13,108 with the default `fov = 20`). Its simulation has no memory: each frame
starts again from the seed sphere. So here the same maths runs in the vertex
shader, only for the particles that are drawn. On the same machine, the original
ran at 8–13 fps without a frame limit, against hundreds for this engine.

**No buffers.** Each particle derives its seed from its index (`gl_VertexID`) with
an integer hash, with the same distribution as the original. Nothing is uploaded
to the GPU.

**Nothing allocated per frame.** Matrices, timing buffers and the scroll driver's
parameter object are reserved once. The size is read only when the canvas is
resized; scene anchors only when the page is resized.

**Cheap rendering.** An opaque canvas (the page is black anyway), no MSAA (it does
nothing for round sprites), the low-power GPU preferred, and a cap on point size
so that very blurred particles do not cost too much.

**Does not block the page.** The shaders are compiled in parallel
(`KHR_parallel_shader_compile`) and checked on the first frame, so the title
paints first.

**Adaptive quality.** Point sizes do not depend on the screen's pixel density, so
the engine can lower the resolution without changing the look: if the median frame
takes more than 18 ms, it goes from DPR 2 to 1.5 and then to 1. If it is still
slow, the cost is in the vertices (many particles), so it lowers `detail` one
octave at a time down to 1: the swirls lose some fine detail, the shape stays. On
screens or modes limited to 30 Hz this also kicks in, which is acceptable.

**Pauses.** It stops when the tab is hidden or the canvas is off-screen, and with
`prefers-reduced-motion` it draws a single still frame.

## Measurements

Apple M5, headless Chromium 1248 with ANGLE Metal, 1440 × 900 at DPR 2, default
parameters, production build (`astro preview`), 2026-11-05.

| What | Result |
|---|---|
| Landing transfer | **3 requests, 14.1 KB brotli** |
| — HTML with inline CSS | 2.4 KB |
| — Engine, scenes and component JS | 5.2 KB |
| — Display font (Latin subset) | 6.6 KB |
| First contentful paint (title) | 24–56 ms |
| FPS without frame limit, landing values (262,144 particles), 3 runs, before the stories | 318, 322, 330 |
| FPS without frame limit, `pmndrs` without dust (13,108 particles), 3 runs | 867, 869, 882 |
| JS allocations while running | ~150 B in 5 s (sampling noise) |
| Rendering with the tab hidden | 0 frames |
| FPS without frame limit at each landing scene (inicio, seda, nube, flor), 3 runs each | 261–271 |
| JS allocations while scrolling through the whole story every frame | ~2 KB in 1,379 frames |

Untested: a real phone. With all 262,144 particles the work is in the vertices,
so a slow phone relies on adaptive quality lowering `detail`.

Before this pass, the same landing was 26.5 KB in 6 requests. The 1.9 MB seen
with `bun run dev` is the development server (unbundled modules and Astro's
toolbar); it never reaches production.

`bun run size` (in `apps/web`, after `bun run build`) prints the per-file table and
fails above **16 KB brotli or 4 requests**.

## Use

```astro
---
import FloresBloom from "../components/bloom/FloresBloom.astro";
---
<FloresBloom class="absolute inset-0" params={{ bloom: 1, rotate: 0, shake: 0, focus: 6 }} />
```

The component does not position itself: the caller gives it a size (above,
`absolute inset-0`). Without WebGL2, or if the shaders fail, it shows a soft static
glow.

## Parameters

See `params.ts`, where each one is documented. Groups:

- **Simulation and depth of field:** `focus`, `speed`, `blur`, `fov` (the fraction
  of particles drawn, not a camera angle; 0 = all), `curl`, `detail`, `seeds`,
  `falloff`, `dust`.
- **Camera:** `lens` (the real field of view), `distance`, `rotate`, `shake`, `x`
  and `y` (screen
  position in half screens; on portrait screens `x` shrinks and is 0 below 3:5,
  so the shape stays on a phone's screen).
- **Flower mode:** `bloom`, `petals`, `layers`, `turbulence`, `cup`, `width`, `spin`.

Every numeric parameter eases towards the value given to `set()` with a time
constant of 0.16 s, the same at 60 or 120 Hz, and the clocks are integrated
(`time += dt · speed`), so `speed` and `rotate` can change without jumps.

Tip: with `rotate: 0` and `shake: 0` the camera faces the flower at `distance`
units, so set `focus` close to `distance` or most of it will be blurred.

## Stories

A story is a list of scenes in `scenes.ts`; the page marks one element per scene
with `data-bloom-scene="<name>"`:

```astro
<FloresBloom class="fixed inset-0" story="landing" />
<section data-bloom-scene="inicio" class="h-dvh">…</section>
<section data-bloom-scene="seda" class="h-dvh">…</section>
```

When a scene's element reaches the top of the viewport, that scene is complete;
between two elements the numeric parameters are interpolated (smoothstep) and the
others switch halfway. `curl` is interpolated geometrically: it reads as a ratio,
so the shape draws itself evenly instead of turning into a cloud early.

The landing's story: **inicio** (the silk shape picked on Maxime's page, in the
centre under the title) → **seda** on the left → **nube** on the right
(`curl: 1`) → **flor** in the centre (`bloom: 1`, closer, in focus and almost
still). It is always in motion (`speed: 34.5`); the side positions leave room for
the content. With
`prefers-reduced-motion`, scrolling still changes the scene, one still frame at a
time.

## Backlog

Noted, not built yet:

- Clicks on the still flower: a pulse, a wave that pushes particles, opening and
  closing.
- Pointer parallax or a pointer that bends the field.
- Transitions triggered by events, not only by scroll.
- One canvas that persists across pages (`transition:persist`) when there are more
  pages.

## Tuning page

`bun run dev:web` and open `/dev/bloom`: a preset selector and one control per
parameter, with FPS, CPU and GPU time per frame. The URL keeps the preset and the
values that differ from it (`?preset=maxime&p={"curl":0.15,"fov":0}`), and
**Copiar parámetros** copies them as JSON. **escena** loads one scene of the
landing story, **progreso** scrubs through it as scrolling would, and **Copiar
escena** copies the current values as an entry of `scenes.ts`. Add `&adaptive=0`
to keep full resolution while measuring. Only the development server serves this
page.
