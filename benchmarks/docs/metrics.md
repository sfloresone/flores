# Métricas

Todas las métricas numéricas se publican con el mismo bloque estadístico:

```json
{ "n": 15, "mean": 20.1, "stddev": 0.8, "min": 19.0, "p50": 20.3, "p95": 21.7, "max": 22.0,
  "ci95_p50": [19.1, 20.8] }
```

`p50` es la cifra principal. `ci95_p50` es el intervalo de confianza del 95 % de
la mediana (2000 remuestreos bootstrap, semilla 0).

## perf-startup

| Métrica | Unidad | Definición |
|---|---|---|
| `wall_ms` | ms | Tiempo real de `<agente> --version`, de `exec` a `exit` (hyperfine). |
| `cpu_ms` | ms | Tiempo de CPU de usuario + sistema. |
| `rss_mb` | MiB | Pico de memoria residente, de 10 ejecuciones aparte bajo `tools/measure` (no de hyperfine; ver [RSS](#rss-y-el-suelo-del-lanzador)). |
| `rss_floor_mb` | MiB | Suelo del lanzador (solo Linux; 0 en macOS). |

El sujeto `process-baseline` (`/usr/bin/true`) marca el mínimo que cuesta
lanzar un proceso en esa máquina.

## perf-hello

| Métrica | Unidad | Definición |
|---|---|---|
| `wall_ms` | ms | Tiempo desde lanzar el agente con "hello world" hasta que termina. |
| `harness_ms` | ms | `wall_ms` menos el tiempo que tardó el mock en responder. |
| `cpu_ms` | ms | CPU de usuario + sistema del proceso (`rusage`, vía `tools/measure`). |
| `rss_mb` | MiB | Pico de RSS del proceso (`ru_maxrss`, normalizado: Linux da KiB y macOS bytes). |
| `rss_floor_mb` | MiB | Suelo del lanzador (solo Linux; 0 en macOS). |
| `request_bytes` | bytes | Suma del cuerpo de todas las peticiones al modelo. Es lo más parecido a "coste de contexto fijo" del harness. |
| `system_bytes` | bytes | Bytes del system prompt: campo `system` en Anthropic, `instructions` en Responses y mensajes `system`/`developer`. |
| `tools` | n | Tools declaradas en la petición (informativo, ver metodología). |
| `llm_requests` | n | Peticiones al modelo que hizo el agente para un solo "hello". |

Una ejecución cuenta como válida si el agente sale con código 0 **y** su
salida contiene la respuesta del guion. Las fallidas se cuentan en
`info.failures` y no entran en las estadísticas.

## Capa C (planificada)

| Métrica | Definición |
|---|---|
| pass@1 | Fracción de tareas resueltas en un intento (media sobre k repeticiones). |
| pass^k | Fracción de tareas resueltas en **las k** repeticiones: mide la fiabilidad. |
| tokens in/out, cache read/write | Sumados desde `session.end.usage` de la trayectoria. |
| coste (USD) | Tokens × precio del modelo en la fecha de la ejecución. |
| turnos, tool calls, tool errors | Desde la trayectoria. |
| tiempo total / harness / LLM | `duration_ms`, `harness_ms` y la suma de `llm.response.duration_ms`. |

## RSS y el suelo del lanzador

En Linux, el `ru_maxrss` de un proceso incluye el pico de RSS del espacio de
memoria al que sustituyó al hacer `exec`. Un agente lanzado directamente desde
Python (o desde hyperfine) declara como mínimo la memoria de su padre. En la
primera CI, todos los agentes medían unos 24 MiB, incluido un stub en Go de
pocos MiB. macOS no tiene este problema.

Por eso los agentes se lanzan con `tools/measure`, un programa pequeño en Go
que ejecuta el comando, lee su `rusage` y anota su propio pico de memoria en el
momento del `exec`: el **suelo** (`rss_floor_mb`, unos 2,5 MiB). Comprobado en
Linux con `flores-stub`:

| Medida | Lanzado desde Python | Con `tools/measure` |
|---|---:|---:|
| `perf-hello` | 23,98 MiB | 6,88 MiB (suelo 2,3) |

Si un valor queda a menos de 0,1 MiB del suelo, no se distingue de él, y el
informe lo muestra como **`≤ suelo`** en vez de dar un número falso. Los tiempos
de `perf-startup` siguen siendo de hyperfine, que es más preciso para
ejecuciones de milisegundos.

