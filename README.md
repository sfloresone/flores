# flores

Un coding agent ("harness") construido desde cero en Go con fines educativos:
lo más ligero, portable y eficiente posible, y agnóstico al modelo y al proveedor.

El proyecto sigue un **TDD de rendimiento**: primero se construye la suite de
benchmarks y después el agente, de modo que cada decisión se justifica con datos.

| Parte | Estado |
|---|---|
| [`benchmarks/`](benchmarks/): suite de benchmarks (Python + helpers en Go) | F1 ✅ |
| Agente `flores` (Go) | pendiente |
| Web (exposición de lo aprendido y resultados) | pendiente |

Resultados actuales: [`benchmarks/published/README.md`](benchmarks/published/README.md).

## Referencias

- [Build your own coding agent](https://www.byoharness.dev/es/chapters) (bettatech): estructura del agente en Go, capítulo a capítulo.
- [fx](https://github.com/vercel-labs/fx) (Vercel): agente mínimo en Zig, agnóstico al proveedor; su carpeta `benchmarks/` inspira la nuestra.
- [crush](https://github.com/charmbracelet/crush) (Charm): referencia de estructura profesional y open source de un binario en Go.
