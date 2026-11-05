# Formato y publicación de resultados

## Decisión: JSON como fuente de verdad, Markdown para leer y MDX en la web

| Formato | Para quién | Interactivo | ¿Se versiona? |
|---|---|---|---|
| **JSON** (`latest.json`, `history.jsonl`) | máquinas: CI, informes, web | — | ✅ fuente de verdad |
| **Markdown** (`published/README.md`) | quien lee el repo en GitHub | gráficas Mermaid estáticas | ✅ generado, nunca a mano |
| **MDX** (en la web, más adelante) | visitantes de la web | ✅ tooltips, filtros, series temporales | la página sí; los datos vienen del JSON |

Motivos:

- **Los datos no van dentro del documento.** Si los números vivieran en un MDX,
  cada ejecución de la CI reescribiría prosa, los diffs serían ilegibles y la
  web no podría reutilizarlos. Con JSON, la CI solo añade datos y el diff de
  `history.jsonl` es una línea por ejecución.
- **GitHub no ejecuta MDX.** Un `.mdx` en el repo se ve como texto plano. En
  cambio, GitHub sí renderiza bloques ```` ```mermaid ```` (`xychart-beta`), así
  que el `README.md` generado ya muestra gráficas de barras sin JavaScript.
- **La web sí ejecuta MDX.** Con un framework de contenido estático como Astro
  o Next.js, un `.mdx` puede importar un componente de gráfica (Recharts,
  Observable Plot, ECharts…) que lea `published/**/latest.json` en el build.
  El resultado es una página estática que solo hidrata la gráfica: barras con
  tooltip, selector de plataforma y evolución por commit desde `history.jsonl`.
  Ese componente se escribirá al montar la web.

## Estructura

```
results/                                    # crudo, en .gitignore (artifacts en CI)
  perf-hello/20261105T133754Z-darwin-arm64.json   # incluye todas las muestras
published/                                  # versionado
  README.md                                 # informe generado (flores-bench report)
  perf-startup/linux-x86_64/latest.json     # última ejecución, sin muestras
  perf-startup/linux-x86_64/history.jsonl   # una línea por ejecución publicada
  perf-hello/darwin-arm64/...
```

## `result/v0`

```jsonc
{
  "schema_version": "result/v0",
  "suite": "perf-hello",
  "created_at": "2026-11-05T13:37:54Z",
  "git": {"sha": "…", "short": "…", "branch": "main", "dirty": false},
  "machine": {"platform": "linux-x86_64", "cpu": "…", "cores": 4, "mem_bytes": …, "ci": true, "runner": "…"},
  "tools": {"python": "3.12.x", "go": "go1.27.1", "hyperfine": "hyperfine 2.0.0", "docker": "…"},
  "params": {"runs_per_round": 5, "warmup": 1, "rounds": 3, "seed": 0, …},
  "subjects": [
    {
      "name": "fx", "label": "fx", "available": true,
      "version": "0.0.16", "pinned_version": null, "version_mismatch": false,
      "model": "openai/gpt-6.1-sol", "skipped_reason": null,
      "metrics": {"wall_ms": {"n": 15, "p50": 20.3, "ci95_p50": [19.1, 20.8], …}, …},
      "info": {"failures": 0, "protocols": ["openai-chat"], …},
      "samples": [ … ]          // solo en results/, nunca en published/
    }
  ]
}
```

Cada línea de `history.jsonl` tiene `{created_at, sha, dirty, cpu, subjects: {name: {version, "<metric>.p50": …}}}`.

## Flujo en la CI (`.github/workflows/bench.yml`)

1. **En un PR**: tests, después `perf startup` y `perf hello` en `ubuntu-latest`,
   con los competidores instalados en sus versiones fijadas, y el control de
   presupuestos. Los resultados crudos se suben como *artifact*.
2. **En un push a `main`**: lo mismo, y además `flores-bench publish` y un
   commit automático de `published/` con `[skip ci]`.
3. **Ejecuciones locales** (macOS u otras): `make bench && make publish` y se
   commitean a mano. Se guardan en su propia carpeta de plataforma y nunca pisan
   la referencia de Linux.
4. **Capa C** (cuesta dinero): solo manual y con commit a mano; nunca en cada PR.
