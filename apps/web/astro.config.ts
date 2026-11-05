import mdx from "@astrojs/mdx";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "astro/config";

export default defineConfig({
  integrations: [
    react(),
    mdx(),
    {
      name: "flores-dev-catalog",
      hooks: {
        "astro:config:setup": ({ command, injectRoute }) => {
          if (command === "dev") {
            injectRoute({ pattern: "/dev", entrypoint: "./src/dev/catalog.astro" });
          }
        },
      },
    },
  ],
  vite: {
    plugins: [tailwindcss()],
  },
  server: { port: 4321 },
});
