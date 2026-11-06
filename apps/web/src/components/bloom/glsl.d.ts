/** `*.glsl` files import as their (build-time minified) source string. See astro.config.ts. */
declare module "*.glsl" {
  const source: string;
  export default source;
}
