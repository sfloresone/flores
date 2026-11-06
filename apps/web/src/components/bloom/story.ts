/**
 * Scroll-driven story: each element with data-bloom-scene="<name>" anchors one
 * scene. When an anchor reaches the top of the viewport its scene is complete;
 * between two anchors the numeric parameters are interpolated (smoothstep;
 * curl geometrically) and the discrete ones switch halfway. The engine's easing adds inertia on top.
 *
 * No layout reads per frame: anchor positions are measured on resize only, and
 * the frame reads scrollY. Nothing is allocated per frame.
 */
import type { Bloom } from "./engine";
import { NUMERIC_KEYS, type BloomParams } from "./params";
import type { Scene } from "./scenes";

/** Writes into `out` the parameters at `progress` (0 = first scene … n - 1 = last). */
export function sample(scenes: Scene[], progress: number, out: BloomParams): BloomParams {
  const last = scenes.length - 1;
  const p = Math.min(Math.max(progress, 0), last);
  const i = Math.min(Math.floor(p), Math.max(last - 1, 0));
  const a = scenes[i]!.params;
  const b = (scenes[i + 1] ?? scenes[i]!).params;
  const t = p - i;
  const s = t * t * (3 - 2 * t);
  Object.assign(out, s < 0.5 ? a : b); // discrete parameters switch halfway
  for (const k of NUMERIC_KEYS) out[k] = a[k] + (b[k] - a[k]) * s;
  // curl reads as a ratio (0.15 → 0.3 changes as much as 0.5 → 1): geometric
  // interpolation spreads the drawing evenly instead of jumping to a cloud early.
  if (a.curl > 0 && b.curl > 0) out.curl = a.curl * (b.curl / a.curl) ** s;
  return out;
}

export interface Story {
  /** Reads the scroll position and sets the matching parameters on the engine. */
  update(): void;
  destroy(): void;
}

export function scrollStory(bloom: Bloom, scenes: Scene[]): Story {
  const anchors = scenes.map((s) => document.querySelector<HTMLElement>(`[data-bloom-scene="${s.name}"]`));
  let tops: number[] = [];
  const out = { ...scenes[0]!.params };

  const measure = () => {
    const y = scrollY;
    tops = anchors.map((el, i) => (el ? el.getBoundingClientRect().top + y : (tops[i - 1] ?? 0)));
  };

  /** Scene progress for the current scroll position. */
  const progress = () => {
    const y = scrollY;
    const last = tops.length - 1;
    if (y <= tops[0]!) return 0;
    for (let i = 0; i < last; i++) {
      const a = tops[i]!, b = tops[i + 1]!;
      if (y < b) return i + (b > a ? (y - a) / (b - a) : 1);
    }
    return last;
  };

  const update = () => bloom.set(sample(scenes, progress(), out));

  // Animated: the engine calls update() from its own frame (onFrame). Still
  // (reduced motion): there is no frame loop, so scrolling draws one frame.
  let pending = 0;
  const onScroll = () => {
    if (!bloom.still || pending) return;
    pending = requestAnimationFrame(() => {
      pending = 0;
      update();
    });
  };
  const ro = new ResizeObserver(() => {
    measure();
    if (bloom.still) update();
  });
  ro.observe(document.documentElement);
  addEventListener("scroll", onScroll, { passive: true });
  measure();
  update();

  return {
    update,
    destroy() {
      ro.disconnect();
      removeEventListener("scroll", onScroll);
      cancelAnimationFrame(pending);
    },
  };
}
