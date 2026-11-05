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
| **B. Comportamiento del harness** | bucle de tools, errores del proveedor, reintentos, streams cortados, límites… con escenarios guionizados | No (mock) | 0 € | ✅ F2 (13 escenarios) |
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
make harness       # capa B: matriz escenario × agente
make publish       # copia los resultados a published/ y regenera el informe
```

O directamente con la CLI:

```sh
uv run flores-bench perf startup --agents flores-stub,fx --rounds 5
uv run flores-bench perf hello --runs 10
uv run flores-bench harness --agents flores-stub,pi --scenarios rate-limit-recover
uv run flores-bench scenarios                # lista los escenarios de la capa B
uv run flores-bench check-budgets results/startup.json
uv run flores-bench trajectory validate run.jsonl
```

## Estructura

```
benchmarks/
├── flores_bench/          # paquete Python: CLI, runners, adaptadores, estadística, informes
│   ├── adapters/          # cómo arrancar y conectar cada agente al mock
│   ├── perf/              # capa A: startup.py, hello.py
│   └── harness/           # capa B: escenarios, checks, runner
├── tools/                 # helpers en Go (mismo lenguaje que flores)
│   ├── mockprovider/      # proveedor LLM falso: OpenAI Chat, OpenAI Responses, Anthropic Messages
│   └── flores-stub/       # agente mínimo que cumple el contrato headless
├── schema/                # trajectory.v0.schema.json: el contrato de salida del agente
├── scenarios/harness/     # capa B: guion del mock + expectativas de cada escenario
├── suites/perf/           # agents.toml (versiones fijadas) y budgets.toml (límites de CI)
├── tests/                 # pytest
├── docs/                  # metodología, métricas, contrato, formato de resultados, adaptadores
├── results/               # resultados crudos (en .gitignore)
└── published/             # resultados compactos versionados + informe generado
```

## Documentación

Índice completo y a quién va dirigido cada documento: [`docs/README.md`](docs/README.md).

- [Glosario](docs/glossary.md): las palabras que usamos, sin jerga.
- [Metodología](docs/methodology.md): qué medimos, cómo y con qué límites.
- [Clasificación de pruebas](docs/test-taxonomy.md): niveles N0–N3 según lo fiable que es cada prueba y qué puede bloquear la CI.
- [Pipeline y arquitectura](docs/pipeline.md): qué hace cada pieza y qué ejecuta la CI.
- [Escenarios de la capa B](docs/harness-scenarios.md): cada fallo guionizado y por qué importa (generado).
- [Métricas](docs/metrics.md): definición exacta de cada número.
- [Contrato headless y trayectorias](docs/trajectory-schema.md): lo que `flores run` debe emitir.
- [Formato y publicación de resultados](docs/results-format.md): JSON → Markdown → web.
- [Adaptadores](docs/adapters.md): cómo se conecta cada agente al mock y lo que descubrimos.
