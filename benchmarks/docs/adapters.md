# Adaptadores: cómo se conecta cada agente al mock

Cada adaptador (`flores_bench/adapters/agents.py`) solo responde a una
pregunta: con qué comando, entorno y ficheros de configuración hace este agente
la tarea X contra el proveedor Y. La medición la hacen los runners, con el
mismo código para todos.

Para todos los agentes:

- Directorio de configuración **nuevo y vacío en cada ejecución**, y workspace vacío.
- Entorno saneado: sin claves reales (`SECRET_ENV`) y sin las variables que
  delatan que corre dentro de otro agente (`CLAUDECODE`, …).
- `stdin` = `/dev/null`. Codex, por ejemplo, se queda esperando stdin si no se cierra.
- Un ID de modelo real que el agente conoce (`suites/perf/agents.toml`).

| Agente | Versión ref. | Protocolo | Cómo se apunta al mock | Comando |
|---|---|---|---|---|
| flores | (código) | Anthropic Messages | `FLORES_PROVIDER=anthropic`, `FLORES_BASE_URL=<mock>`, `FLORES_API_KEY=mock` | `flores -p … --output jsonl` |
| flores-stub | — | OpenAI Chat | `FLORES_BASE_URL=<mock>/v1` | `flores-stub run --prompt …` |
| Pi | 1.5.0 | OpenAI Responses | `PI_CODING_AGENT_DIR` con `models.json` que solo cambia `baseUrl` del proveedor `openai`; `PI_OFFLINE=1` | `pi -p --model openai/gpt-6.1-sol --no-session …` |
| Claude Code | 2.1.318 | Anthropic Messages | `CLAUDE_CONFIG_DIR`, `ANTHROPIC_BASE_URL=<mock>`, `ANTHROPIC_API_KEY=mock`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` | `claude -p --model claude-sonnet-5-5 …` |
| Codex | 0.164.0 | OpenAI Responses | `CODEX_HOME` con `config.toml`: `model_provider = "mock"`, `wire_api = "responses"` | `codex exec --skip-git-repo-check …` |
| fx | 0.0.16 | OpenAI Chat | `HOME` aislado con `.fx/settings.json`: conexión `openai-chat-completions` y `FX_PROVIDER=mock` | `fx ask --no-save …` |

## Cosas que descubrimos al conectarlos (y por qué importan)

1. **fx usó la suscripción del usuario en vez del mock.** La documentación
   actual de fx describe `FX_HOME`, pero la 0.0.16 no lo soporta: leía
   `~/.fx`, encontraba el login de ChatGPT/Codex y respondía "Hello, world!"
   desde OpenAI (unos 5 s) en lugar de "Hello! Nice to meet you." desde el mock
   (unos 20 ms). Lecciones:
   - **Comprobar siempre la respuesta.** `perf-hello` exige el texto exacto del
     guion; si no aparece, la ejecución no cuenta.
   - **Aislar `HOME`**, no solo la variable que diga la documentación.
2. **Claude Code cambia el system prompt según el modelo.** Con un ID que no
   conoce avisa (`unrecognized_model`) y usa otro prompt (5,8 KB frente a
   8,7 KB). Por eso usamos IDs reales.
3. **Las tools no siempre van en el campo `tools`.** Con modelos `gpt-6.x` en
   la API Responses, Pi y Codex envían las definiciones como un item
   `additional_tools` dentro de `input`, y Codex además las agrupa en
   *namespaces* (`functions`, `clock`, `collaboration`). El mock las cuenta en
   ambos sitios desde la F2; en la F1 aparecían como 0 tools. Aun así, la
   métrica más comparable es `request_bytes`.
4. **`--version` no es representativo.** Claude Code tarda unos 9 ms en
   `--version` y unos 167 ms en un turno completo.

## Ver exactamente qué envía cada agente

```sh
.bin/mockprovider --port 53000 --dump-bodies /tmp/dump &
# lanza el agente apuntando a http://127.0.0.1:53000
ls /tmp/dump   # 000001.json, ... : el cuerpo exacto de cada petición
```

Es la forma más rápida de estudiar el system prompt, las definiciones de tools
y la gestión del historial de cada harness. Material de aprendizaje directo
para diseñar los de flores.

## Añadir un agente

1. Crear una subclase de `Adapter` en `adapters/agents.py` con `name`,
   `binary`, `version_args`, `invoke()` (respetando `full_access`) y, si se
   conoce su tool de shell, `shell_call()`.
2. Registrarla en `ADAPTERS` y añadirla a `suites/perf/agents.toml` (modelo y,
   si se instala por npm, `npm` y `version`).
3. `uv run flores-bench perf hello --quick --agents <nombre>` y comprobar que
   `info.unknown_paths` está vacío y que no hay `failures`.
4. `uv run flores-bench harness --agents <nombre>` y revisar cada ❌: ¿es un
   comportamiento real del agente o un fallo del adaptador?

## Tool de shell de cada agente (capa B)

Los escenarios escriben `{"shell": "<comando>"}` y cada adaptador lo traduce.
Se descubrió volcando la primera petición de cada agente (`--dump-bodies`).

| Agente | Tool | Argumentos | Modo sin permisos usado en la capa B |
|---|---|---|---|
| flores | `bash` | `{"command"}` (M0 no tiene tools: responde con error) | — |
| flores-stub | `bash` | `{"command"}` (no la ejecuta: responde con error) | — |
| Pi | `bash` | `{"command", "timeout?"}` | no tiene permisos en `-p` |
| Claude Code | `Bash` | `{"command", "description", "timeout?"}` | `--dangerously-skip-permissions` |
| fx | `shell` | `{"action": "run", "command"}` | `--full-access` |
| Codex | `functions.exec` (namespace) | tool de texto libre, sin esquema JSON | pendiente: los escenarios con tools salen como "no aplicable" |

Todo corre en un workspace temporal y los comandos de los escenarios solo hacen `echo`.

## Comportamientos observados en la capa B

Resultados completos en [`published/README.md`](../published/README.md). Lo más instructivo:

- **Nadie limita los pasos por defecto en modo headless.** En `runaway-tool-loop`
  (el modelo pide tools para siempre) Pi, Claude Code, Codex y fx hicieron entre
  3000 y 7000 peticiones en 45 s hasta que el runner los mató. Con un modelo real
  eso es dinero sin techo. flores-stub para a los 8 pasos (`--max-steps`).
  flores no tiene límite por defecto, pero detecta la repetición y para en 6
  peticiones (5 pasos y uno de resumen; [ADR 0003](../../docs/adr/0003-bucle-del-agente.md)).
- **`Retry-After` largo: casi nadie lo gestiona bien.** En `retry-after-too-long`
  (429 pidiendo esperar una hora) solo Claude Code y flores fallan en el acto.
  - fx recorta la espera a 30 s y vuelve a pedir.
  - Pi ignora la cabecera y reintenta a los 2, 4 y 8 s (4 peticiones).
  - Codex se queda esperando en silencio.

  Medido en local (macOS); los datos de referencia los publica la CI de Linux.
  Detalle en el [ADR 0004](../../docs/adr/0004-reintentos.md).
- **Sin saldo, fx sigue reintentando.** En `out-of-credit-fail-fast`, OpenAI
  responde un 429 `insufficient_quota`, el mismo código que el de límite de
  peticiones; en Anthropic llega como un 400 "credit balance is too low".
  - Pi, Claude Code, Codex y flores (con los dos protocolos) fallan a la primera.
  - fx hace 7 peticiones en 60 s, hasta que el runner lo mata.

  Medido en local (macOS). Detalle en el [ADR 0005](../../docs/adr/0005-adaptador-openai.md).
- **401 no siempre falla rápido.** Pi y fx abandonan a la primera. Codex hace 6
  intentos (≈6 s) y Claude Code sigue reintentando con backoff durante más de
  60 s. Una clave inválida no se arregla reintentando.
- **Pi compacta el contexto tras una respuesta de 1 MiB** (una segunda petición
  con un prompt de "context summarization assistant"), incluso en `-p --no-session`,
  donde la sesión termina justo después.
- **Todos se recuperan de 429, 503 y streams cortados**, y todos devuelven al
  modelo el error de una tool inexistente o con JSON roto en lugar de caerse.
