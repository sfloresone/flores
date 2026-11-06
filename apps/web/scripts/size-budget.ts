/**
 * Transfer budget for the landing: the HTML plus every asset it requests at load
 * (scripts, stylesheets, preloads, icons), compressed with brotli and gzip as a
 * CDN would. Deterministic: the same dist gives the same numbers.
 *
 *   bun run build && bun run size
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

const BUDGET_BROTLI_BYTES = 16 * 1024;
const BUDGET_REQUESTS = 4;

const dist = join(import.meta.dir, "..", "dist");
const html = readFileSync(join(dist, "index.html"));

// Same-origin URLs referenced by the tags the browser fetches during load.
const refs = new Set<string>();
for (const tag of html.toString().matchAll(/<(?:script|link)\b[^>]*>/g)) {
  const t = tag[0];
  if (t.startsWith("<link") && !/rel="?(?:stylesheet|preload|modulepreload|icon)/.test(t)) continue;
  const url = /(?:src|href)="?([^"\s>]+)/.exec(t)?.[1];
  if (url?.startsWith("/")) refs.add(url);
}

const kb = (n: number) => (n / 1024).toFixed(2).padStart(6);
const br = (b: Buffer) =>
  brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }).length;
const isCompressed = (p: string) => /\.(woff2|png|jpe?g|webp|avif)$/.test(p);

let totalBr = 0, totalGz = 0;
console.log("     raw     gzip   brotli  (KB)");
for (const path of ["/index.html", ...refs]) {
  const file = path === "/index.html" ? html : readFileSync(join(dist, path));
  const g = isCompressed(path) ? file.length : gzipSync(file, { level: 9 }).length;
  const b = isCompressed(path) ? file.length : br(file);
  totalBr += b;
  totalGz += g;
  console.log(`${kb(file.length)}   ${kb(g)}   ${kb(b)}  ${path}`);
}
const requests = refs.size + 1;
console.log(`total: ${kb(totalGz)} KB gzip · ${kb(totalBr)} KB brotli · ${requests} requests`);

const over: string[] = [];
if (totalBr > BUDGET_BROTLI_BYTES) over.push(`brotli ${kb(totalBr)} KB > ${kb(BUDGET_BROTLI_BYTES)} KB`);
if (requests > BUDGET_REQUESTS) over.push(`${requests} requests > ${BUDGET_REQUESTS}`);
if (over.length) {
  console.error(`over budget: ${over.join(", ")}`);
  process.exit(1);
}
console.log(`within budget (≤ ${kb(BUDGET_BROTLI_BYTES).trim()} KB brotli, ≤ ${BUDGET_REQUESTS} requests)`);
