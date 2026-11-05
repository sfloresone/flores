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

The landing uses `maxime` with frequency 0.15, speedFactor 34.5, fov 0, blur 0.9
and focus 6.90: all 262,144 particles, small and sharp.

## How it works, in four ideas

1. **Particles start on a sphere.** Each one has a fixed "seed" point.
2. **Curl noise moves them.** Curl noise is a smooth field that looks like smoke or
   fluid. The position of each particle is a function of its seed and the time.
3. **Fake depth of field.** The further a particle is from the focus distance, the
   bigger and more transparent it is drawn. There is no post-processing.
4. **Flower mode** (`petals > 0`). The petals are built geometrically (each particle
   picks a petal, a place across it and a place from base to tip) and the curl
   motion of the original is added on top as turbulence.

## Files

| File | What it is |
|---|---|
| `FloresBloom.astro` | The component: a canvas, the fallback and a few lines that start the engine. |
| `engine.ts` | WebGL setup, frame loop, pausing, adaptive quality. |
| `vertex.glsl`, `fragment.glsl` | The shaders. Minified at build time (see `astro.config.ts`). |
| `camera.ts` | The original's camera, without allocations. |
| `params.ts` | Parameters, defaults and ranges. |

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

**Nothing allocated per frame.** Matrices and timing buffers are reserved once.
The size is read only when the canvas is resized, and uniforms that only change
with `set()` are sent only then.

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
| Landing transfer | **3 requests, 13.6 KB brotli** |
| — HTML with inline CSS and the parameters | 2.5 KB |
| — Engine + component JS | 4.5 KB |
| — Display font (Latin subset) | 6.6 KB |
| First contentful paint (title) | 24–56 ms |
| FPS without frame limit, landing values (262,144 particles), 3 runs | 318, 322, 330 |
| FPS without frame limit, `pmndrs` without dust (13,108 particles), 3 runs | 867, 869, 882 |
| JS allocations while running | ~150 B in 5 s (sampling noise) |
| Rendering with the tab hidden | 0 frames |

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
<FloresBloom class="absolute inset-0" params={{ petals: 6, view: "front", focus: 6 }} />
```

The component does not position itself: the caller gives it a size (above,
`absolute inset-0`). Without WebGL2, or if the shaders fail, it shows a soft static
glow.

## Parameters

See `params.ts`, where each one is documented. Groups:

- **Simulation and depth of field:** `focus`, `speed`, `blur`, `fov` (the fraction
  of particles drawn, not a camera angle; 0 = all), `curl`, `detail`, `seeds`,
  `falloff`, `dust`.
- **Camera:** `lens` (the real field of view), `distance`, `rotate`, `shake`.
- **Flower mode:** `view`, `petals`, `layers`, `turbulence`, `cup`, `width`, `spin`.

Tip: in `view: "front"` the flower sits about 6 units from the camera, so set
`focus` close to 6 or most of it will be blurred and transparent.

## Tuning page

`bun run dev:web` and open `/dev/bloom`: a preset selector and one control per
parameter, with FPS, CPU and GPU time per frame. The URL keeps the preset and the
values that differ from it (`?preset=maxime&p={"curl":0.15,"fov":0}`), and
**Copiar parámetros** copies them as JSON. Add `&adaptive=0` to keep full resolution while
measuring. Only the development server serves this page.
