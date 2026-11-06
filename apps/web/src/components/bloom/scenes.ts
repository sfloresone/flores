/**
 * Stories: ordered scenes that Flores Bloom moves through as the page scrolls.
 * Each scene is a full set of parameters; numeric ones are interpolated between
 * neighbours (see story.ts). Tune them in /dev/bloom ("escena", "progreso").
 */
import { PRESETS, type BloomParams } from "./params";

export interface Scene {
  name: string;
  params: BloomParams;
}

/**
 * The landing's base: Maxime's preset with the values picked on his page
 * (frequency 0.15, speedFactor 34.5, fov 0, blur 0.9, focus 6.90), no orbit.
 */
const LANDING_BASE: BloomParams = {
  ...PRESETS.maxime,
  curl: 0.15,
  speed: 34.5,
  fov: 0,
  blur: 0.9,
  focus: 6.9,
  rotate: 0,
  turbulence: 0.15,
};

const scenes = (base: BloomParams, list: { name: string; params: Partial<BloomParams> }[]): Scene[] =>
  list.map(({ name, params }) => ({ name, params: { ...base, ...params } }));

export const STORIES = {
  /**
   * The harness arrives as silk in the centre, under the title; scrolling moves
   * it aside for the content (left, then right) and it opens as a flower in the
   * centre. Always in motion, at the speed picked on Maxime's page.
   */
  landing: scenes(LANDING_BASE, [
    { name: "inicio", params: {} },
    { name: "seda", params: { x: -0.45 } },
    // Maximum frequency: a full cloud, on the right.
    { name: "nube", params: { x: 0.45, curl: 1 } },
    // The flower opens, seen from the front, in focus and almost still.
    { name: "flor", params: { bloom: 1, distance: 3.4, focus: 3.4, turbulence: 0.05 } },
  ]),
} satisfies Record<string, Scene[]>;

export type StoryName = keyof typeof STORIES;

/** What each story's scenes are written against: "Copiar escena" in /dev/bloom diffs against it. */
export const STORY_BASES: Record<StoryName, BloomParams> = { landing: LANDING_BASE };
