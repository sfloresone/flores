# Metodología

## Principio: separar el harness del modelo

Un coding agent tiene dos partes que se miden de forma muy distinta:

- **El harness** (flores): proceso, bucle del agente, tools, contexto, streaming.
  Es determinista y barato de medir si le quitamos el modelo.
- **El modelo**: decide qué hacer. Es caro y no determinista.

Si mezclamos las dos, un cambio de prompt que "mejora" el resultado puede ocultar
que el binario es más lento o que envía el doble de tokens. Por eso hay tres
capas, y las dos primeras **no usan ningún modelo real**.

## Capa A: sistema / perf

Todo corre contra `tools/mockprovider`, un servidor local que habla los tres
protocolos que usan los harnesses actuales (OpenAI Chat Completions, OpenAI
Responses y Anthropic Messages, con y sin streaming) y contesta desde un guion.
Es la misma técnica que el `fake-inference-server.zig` de fx.

| Suite | Qué ejecuta | Herramienta |
|---|---|---|
| `perf-startup` | `<agente> --version` | hyperfine (`-N`, sin shell) |
| `perf-hello` | `"hello world"` de principio a fin; el mock contesta al instante | runner propio (`os.wait4`) |

`perf-hello` no usa hyperfine porque cada ejecución necesita su propio
directorio de configuración y hay que leer el registro del mock entre una
ejecución y la siguiente.

### Rigor de medición

- **Rondas barajadas.** Cada suite se repite en `--rounds` rondas (3 por
  defecto) con los agentes en orden aleatorio y semilla fija. Así, si la máquina
  se calienta o hay un proceso en segundo plano, el efecto se reparte entre
  todos (fx hace lo mismo).
- **Calentamiento (warmup)** antes de medir, para llenar cachés de disco y del runtime.
- **Estadística robusta.** Se publica la mediana (p50) y el p95, no solo la
  media, porque los tiempos de proceso tienen colas largas. Junto a la mediana
  va su **intervalo de confianza del 95 %** calculado por bootstrap con semilla
  fija. Si dos intervalos se solapan, no hay diferencia demostrada.
- **Aislamiento.** Cada ejecución usa un directorio de configuración vacío y un
  workspace vacío, y nunca toca tu `~/.claude`, `~/.codex`, `~/.pi` o `~/.fx`.
- **Credenciales fuera.** El runner borra del entorno las variables con claves
  reales (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, Cloudflare…) antes de lanzar
  un agente. Si un adaptador está mal configurado, el agente falla en lugar de
  gastar dinero. Ver el caso de fx en [adapters.md](adapters.md).
- **Versiones fijadas.** `suites/perf/agents.toml` fija la versión de cada
  competidor. Si la versión instalada es otra, se mide igual pero el informe lo
  marca con ⚠.
- **Metadatos completos.** Cada resultado guarda el commit, si había cambios
  sin commitear, la CPU, la memoria, el SO y las versiones de go, hyperfine y python.

### Plataforma de referencia

**Linux x86_64 en GitHub Actions (`ubuntu-latest`)**. Solo ahí se aplican los
presupuestos (`suites/perf/budgets.toml`), igual que en fx. En macOS y en
portátiles se mide y se publica, pero solo a título informativo: los tiempos
dependen de la gestión térmica y de energía y son más ruidosos.

### Limitaciones conocidas (para leer bien los números)

- **`--version` puede engañar.** Algunos CLIs lo resuelven por un camino rápido
  que no carga el runtime completo. Claude Code tarda unos 9 ms en `--version`
  y unos 167 ms en hacer un turno. La cifra que representa la experiencia real
  es la de `perf-hello`.
- **El RSS es el del proceso lanzado.** `os.wait4` devuelve el pico de RSS del
  hijo directo; si el agente lanza subprocesos y no los espera, su memoria no
  cuenta. Lo mismo ocurre con hyperfine y `/usr/bin/time`.
- **`harness_ms`** = tiempo total − tiempo que tardó el mock en contestar. Como
  el mock es instantáneo, en `perf-hello` es prácticamente igual al total.
- **"Tools" es informativo.** Con modelos que admiten *tool search*, algunos
  harnesses (Pi y Codex con la API Responses) no declaran las tools en la
  primera petición. La métrica comparable es **bytes enviados al modelo**.
- **El modelo configurado cambia el harness.** Claude Code usa un system prompt
  distinto según el modelo. Por eso cada agente usa un ID de modelo real que
  conoce (`agents.toml`) aunque responda el mock.

## Capa B: comportamiento del harness

Mismo mock, con guiones (`scenarios/harness/*.json`) que reproducen los fallos
típicos de un harness: tool calls encadenadas y en paralelo, tools inexistentes,
argumentos con JSON roto, 429 y 5xx transitorios, 401, streams cortados a mitad,
primer token lento, salidas de 1 MiB y bucles de tools sin fin. Lista completa y
el porqué de cada uno: [harness-scenarios.md](harness-scenarios.md).

Cómo se juzga:

- **Solo desde fuera**: código de salida, stdout y lo que el mock observa
  (peticiones, reintentos por turno, resultados de tool devueltos y el cuerpo
  de cada petición). Así el mismo escenario juzga a flores y a cualquier
  competidor, sin depender de su formato interno.
- **Prueba de ejecución real**: los comandos imprimen un marcador calculado
  (`echo flores-$((40+2))`); el escenario exige que `flores-42` llegue al modelo
  en una petición posterior. No basta con que el agente "diga" que ejecutó.
- **Tool calls canónicas**: el guion escribe `{"shell": "…"}` y cada adaptador
  lo traduce a su tool (`Bash`, `bash`, `shell`). Si no se conoce la traducción,
  el escenario es "no aplicable" (—), nunca un fallo.
- **Fallos con estado**: el mock cuenta los intentos por turno, así que puede
  fallar las N primeras veces (`fail_first`) o cortar el stream solo en el
  primer intento (`truncate`) y comprobar que el agente se recupera.
- Para agentes que emiten trayectoria (flores, flores-stub) se valida además
  la trayectoria `trajectory/v0`.

Las expectativas son **la especificación de flores**. Los competidores sirven
de referencia de cómo lo resuelven los harnesses actuales. Es determinista
(nivel N1, ver [test-taxonomy.md](test-taxonomy.md)), así que la puerta de
`budgets.toml` (`[harness.<agente>]`, `must_pass` / `min_passed`) se aplica en
cualquier plataforma y funciona como trinquete: un escenario que flores ya
supera no puede volver a fallar.

## Capa C: tareas end-to-end (planificada)

Modelos reales a través de **Cloudflare AI Gateway**, con un presupuesto de unos 20 €/mes:

1. `flores-micro`: 20–30 tareas propias en Docker con su test de verificación.
   Es el smoke test barato.
2. Un subconjunto de **Terminal-Bench 2.0** (vía Harbor).
3. Un subconjunto pequeño de **SWE-bench Verified** y de **Aider Polyglot**.

Reglas para la capa C:

- k ≥ 3 repeticiones por tarea; se reportan pass@1 y pass^k.
- Comparación pareada: mismas tareas y mismo modelo.
- **Caché del gateway desactivada** durante las evaluaciones, para que una
  repetición no reciba una respuesta cacheada.
- Primero se calibra el coste por tarea con una ejecución pequeña y solo
  después se decide el tamaño de la suite.
- Cuando haya modelos locales (Ollama), servirán para el día a día y los
  modelos frontera quedarán para los checkpoints.
