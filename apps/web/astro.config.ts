import { readFile } from "node:fs/promises";
import mdx from "@astrojs/mdx";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "astro/config";

/**
 * Minify GLSL: keep preprocessor lines and `//!` credit lines (as one block
 * comment each), drop the other comments and every space the grammar does not
 * need. Avoid `- -` in sources: it would collapse into `--`.
 */
export function minifyGlsl(src: string): string {
  let head = "";
  let body = "";
  for (const raw of src.split("\n")) {
    const line = raw.trim();
    if (line.startsWith("#")) head += `${line}\n`;
    else if (line.startsWith("//!")) head += `/*${line.slice(3)} */\n`;
    else body += ` ${line.replace(/\/\/.*/, "")}`;
  }
  return head + body.replace(/\s+/g, " ").replace(/\s*([{}()[\];,=+\-*/<>!?:&|^%])\s*/g, "$1").trim();
}

export default defineConfig({
  build: {
    // The landing has little CSS: inline it instead of a render-blocking request.
    inlineStylesheets: "always",
  },
  integrations: [
    react(),
    mdx(),
    {
      name: "flores-dev-catalog",
      hooks: {
        "astro:config:setup": ({ command, injectRoute }) => {
          if (command === "dev") {
            injectRoute({ pattern: "/dev", entrypoint: "./src/dev/catalog.astro" });
            injectRoute({ pattern: "/dev/tipografia", entrypoint: "./src/dev/typography.astro" });
            injectRoute({ pattern: "/dev/bloom", entrypoint: "./src/dev/bloom.astro" });
          }
        },
      },
    },
  ],
  vite: {
    plugins: [
      tailwindcss(),
      {
        name: "flores-glsl",
        enforce: "pre",
        async load(id) {
          if (!id.endsWith(".glsl")) return null;
          return `export default ${JSON.stringify(minifyGlsl(await readFile(id, "utf8")))};`;
        },
      },
    ],
  },
  server: { port: 4321 },
});
