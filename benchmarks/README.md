# flores-bench

Suite de benchmarks de **flores**. La construimos *antes* que el agente para que
cada mejora se demuestre con datos y no con opiniones: es una especie de TDD de
rendimiento. Primero fijamos qué medimos y cómo; después el agente tiene que
mover esos números.

Resultados publicados: [`published/README.md`](published/README.md).

## Las tres capas

| Capa | Qué mide | ¿LLM real? | Coste | Estado |
|---|---|---|---|---|
| **A. Sistema / perf** | arranque, RSS, CPU, overhead del harness por turno y bytes enviados al modelo | No (mock) | 0 € | ✅ F1 |
| **B. Comportamiento del harness** | bucle de tools, errores del proveedor, reintentos, compactación… con escenarios scripted | No (mock) | 0 € | 🟡 base lista (escenarios) |
| **C. Tareas end-to-end** | tasa de éxito, tokens, coste y tiempo en tareas reales (micro-suite propia, Terminal-Bench, SWE-bench) | Sí | € | ⏳ F3–F4 |

Detalles en [`docs/methodology.md`](docs/methodology.md).

## Requisitos

- Go ≥ 1.25, [uv](https://docs.astral.sh/uv/) y [hyperfine](https://github.com/sharkdp/hyperfine) ≥ 1.19 (la referencia es la 2.0.0).
- Opcional: los agentes con los que comparar (Pi, Claude Code, Codex, fx). Si alguno no está instalado, se marca como "no medido".

```sh
brew install uv hyperfine        # macOS
cd benchmarks
uv sync
uv run flores-bench doctor       # qué está instalado y en qué versión
```

## Uso

```sh
make test          # tests Go + Python + lint
make bench-quick   # pasada rápida de todas las suites de perf (segundos)
make bench         # pasada completa (3 rondas barajadas)
make publish       # copia los resultados a published/ y regenera el informe
```

O directamente con la CLI:

```sh
uv run flores-bench perf startup --agents flores-stub,fx --rounds 5
uv run flores-bench perf hello --runs 10
uv run flores-bench check-budgets results/startup.json
uv run flores-bench trajectory validate run.jsonl
```

## Estructura

```
benchmarks/
├── flores_bench/          # paquete Python: CLI, runners, adaptadores, estadística, informes
│   ├── adapters/          # cómo arrancar y conectar cada agente al mock
│   └── perf/              # capa A: startup.py, hello.py
├── tools/                 # helpers en Go (mismo lenguaje que flores)
│   ├── mockprovider/      # proveedor LLM falso: OpenAI Chat, OpenAI Responses, Anthropic Messages
│   └── flores-stub/       # agente mínimo que cumple el contrato headless
├── schema/                # trajectory.v0.schema.json: el contrato de salida del agente
├── scenarios/             # guiones del mock (capa B)
├── suites/perf/           # agents.toml (versiones fijadas) y budgets.toml (límites de CI)
├── tests/                 # pytest
├── docs/                  # metodología, métricas, contrato, formato de resultados, adaptadores
├── results/               # resultados crudos (en .gitignore)
└── published/             # resultados compactos versionados + informe generado
```

## Documentación

- [Metodología](docs/methodology.md): qué medimos, cómo y con qué límites.
- [Métricas](docs/metrics.md): definición exacta de cada número.
- [Contrato headless y trayectorias](docs/trajectory-schema.md): lo que `flores run` debe emitir.
- [Formato y publicación de resultados](docs/results-format.md): JSON → Markdown → web.
- [Adaptadores](docs/adapters.md): cómo se conecta cada agente al mock y lo que descubrimos.
