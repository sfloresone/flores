# Contrato headless y trayectorias (`trajectory/v1`)

Este es el contrato que el binario `flores` tiene que cumplir para que la suite
pueda medirlo. `tools/flores-stub` ya lo cumple; es la referencia ejecutable.

Vocabulario ([ADR 0003](../../docs/adr/0003-bucle-del-agente.md)): un **turno**
va de un mensaje del usuario a la respuesta final; un **paso** es una llamada al
modelo. Un turno tiene uno o más pasos.

## Invocación

```sh
flores -p PROMPT [--output text|jsonl] [--base-url URL] [--model ID]
                 [--api-key KEY] [--max-steps N] [--timeout DUR]
flores-stub run [--prompt TEXT] [mismos flags]
```

- Si no se pasa el prompt, se lee de stdin.
- `--max-steps N` limita las llamadas al modelo. En `flores`, `0` (por defecto)
  significa sin límite; el stub usa 8.
- Las variables `FLORES_BASE_URL`, `FLORES_MODEL` y `FLORES_API_KEY` sirven como
  valores por defecto de esos flags.
- `--output text` escribe solo la respuesta final en stdout y los diagnósticos en stderr.
- `--output jsonl` escribe en stdout **un evento JSON por línea** (la trayectoria).

| Código de salida | Significado |
|---|---|
| 0 | éxito |
| 2 | uso incorrecto (flags o prompt vacío) |
| 3 | error del proveedor (HTTP ≥ 300, red, stream roto) |
| 4 | paró un freno: `--max-steps` o la detección de repeticiones |

## Eventos

Esquema formal: [`schema/trajectory.v1.schema.json`](../schema/trajectory.v1.schema.json)
(JSON Schema 2020-12). Todos los eventos llevan:

| Campo | Tipo | |
|---|---|---|
| `seq` | int | empieza en 0 y sube de 1 en 1 |
| `t_ms` | number | ms desde el inicio de la sesión (reloj monotónico), nunca decrece |
| `type` | string | uno de los tipos de abajo |

| `type` | Campos obligatorios | Cuándo |
|---|---|---|
| `session.start` | `schema_version`, `agent{name,version}`, `model`, `provider{name,protocol?,base_url?}`, `cwd`, `started_at` | primer evento, una sola vez |
| `step.start` | `step` (≥ 1) | antes de cada llamada al modelo |
| `llm.request` | `step`, `request_bytes`, `messages`, `tools`, `system_bytes` (+ `attempt`) | al enviar la petición |
| `llm.response` | `step`, `status` (`ok`/`error`), `duration_ms` (+ `ttft_ms`, `http_status`, `stop_reason`, `usage`) | al terminar la respuesta |
| `message` | `step`, `role`, `text` | texto del asistente |
| `tool.call` | `step`, `id`, `name`, `arguments` | el modelo pide una tool |
| `tool.result` | `step`, `id`, `ok`, `duration_ms`, `output_bytes` (+ `error`) | la tool terminó; `id` debe existir en un `tool.call` anterior |
| `context.compact` | `before_tokens`, `after_tokens` (+ `strategy`) | se compactó el historial |
| `error` | `kind` (`provider`/`tool`/`limit`/`internal`), `message`, `retryable` (+ `attempt`, `retry_in_ms`) | algo falló |
| `session.end` | `status` (`success`/`error`/`max_steps`/`repetition`/`cancelled`), `steps`, `duration_ms`, `usage`, `tool_calls`, `tool_errors` (+ `final_text`) | último evento, una sola vez |

`usage` = `{input_tokens, output_tokens, cache_read_tokens?, cache_write_tokens?}`.

Cuando para un freno (`max_steps` o `repetition`), `flores` hace **un paso más**
sin tools en el que avisa al modelo del motivo y le pide un resumen de lo hecho y
lo pendiente. Ese paso aparece en la trayectoria como cualquier otro y cuenta en
`steps`; el resumen llega en `final_text`.

**Reintentos** ([ADR 0004](../../docs/adr/0004-reintentos.md)). Un reintento
reenvía la misma petición dentro del **mismo paso**: no hay `step.start` nuevo.
Cada intento fallido deja tres eventos:
- un `llm.request` con su `attempt`;
- un `llm.response` con `status: "error"`;
- un `error` con `retryable: true`, el `attempt` que falló y `retry_in_ms`, la
  espera antes del siguiente.

Si el agente se rinde, el `error` final no lleva `retry_in_ms`. El banco cuenta
los reintentos como peticiones de más en un mismo paso.

Ejemplo (salida real de `flores-stub` contra el escenario `tool-roundtrip`, abreviada):

```jsonl
{"seq":0,"t_ms":0.01,"type":"session.start","schema_version":"trajectory/v1","agent":{"name":"flores-stub","version":"0.0.0-stub"},"model":"mock-model",...}
{"seq":1,"t_ms":0.02,"type":"step.start","step":1}
{"seq":2,"t_ms":0.05,"type":"llm.request","step":1,"request_bytes":186,"messages":2,"tools":0,"system_bytes":77,"attempt":1}
{"seq":3,"t_ms":1.2,"type":"llm.response","step":1,"status":"ok","ttft_ms":1.1,"duration_ms":1.2,"stop_reason":"tool_calls","usage":{...}}
{"seq":4,"t_ms":1.2,"type":"tool.call","step":1,"id":"call_1_0","name":"bash","arguments":{"command":"ls"}}
{"seq":5,"t_ms":1.2,"type":"tool.result","step":1,"id":"call_1_0","ok":false,"duration_ms":0,"output_bytes":52,"error":"tool not implemented"}
...
{"seq":10,"t_ms":2.4,"type":"session.end","status":"success","steps":2,"duration_ms":2.4,"usage":{...},"tool_calls":1,"tool_errors":1,"final_text":"There are no files here."}
```

## Validación

```sh
uv run flores-bench trajectory validate run.jsonl
```

Además del JSON Schema, el validador comprueba que el primer y el último evento
sean `session.start` y `session.end` (y que haya exactamente uno de cada), que
`seq` sea consecutivo, que `t_ms` no retroceda y que cada `tool.result`
responda a un `tool.call` anterior.

## Versionado

Los cambios incompatibles crean una versión nueva en un fichero de esquema
nuevo. Añadir campos opcionales no rompe la versión actual.

- **v1** (actual): `turn` pasó a llamarse `step` (`turn.start` → `step.start`,
  `turns` → `steps`, `max_turns` → `max_steps`) y se añadió el estado
  `repetition`. La v0 llamaba "turno" a lo que en realidad es un paso.
- **v0**: primera versión, solo la emitía `flores-stub`. Se retiró sin periodo de
  convivencia porque no había otros productores.
