# flores

Un coding agent ("harness") construido desde cero en Go con fines educativos:
lo más ligero, portable y eficiente posible, y agnóstico al modelo y al proveedor.

El proyecto sigue un **TDD de rendimiento**: primero se construye la suite de
benchmarks y después el agente, de modo que cada decisión se justifica con datos.

| Parte | Estado |
|---|---|
| [`benchmarks/`](benchmarks/): suite de benchmarks (Python + helpers en Go) | F1 ✅ capa A (perf) · F2 ✅ capa B (comportamiento) |
| [`apps/web`](apps/web): sitio público (Astro) | scaffold |
| `apps/cli`: agente `flores` (Go) | pendiente |

Resultados actuales: [`benchmarks/published/README.md`](benchmarks/published/README.md).

Decisiones de arquitectura del agente: [`docs/adr/`](docs/adr/).

## Estructura

```text
apps/web/       sitio público (aprendizaje + resultados)
benchmarks/     banco de pruebas (fuente de verdad de los números)
docs/           Diátaxis del monorepo
packages/       librerías compartidas (cuando hagan falta)
```

## Web (desarrollo)

Requisitos: Bun 1.4 y **Node.js 22** (Astro / `astro check` no van bien con Node 26).

```sh
bun install
bun run dev:web    # http://localhost:4321
bun run build
```

Requisitos del banco: ver [`benchmarks/README.md`](benchmarks/README.md).

## Referencias

- [Build your own coding agent](https://www.byoharness.dev/es/chapters) (bettatech): estructura del agente en Go, capítulo a capítulo.
- [fx](https://github.com/vercel-labs/fx) (Vercel): agente mínimo en Zig, agnóstico al proveedor; su carpeta `benchmarks/` inspira la nuestra.
- [crush](https://github.com/charmbracelet/crush) (Charm): referencia de estructura profesional y open source de un binario en Go.
