# Contrato headless y trayectorias (`trajectory/v0`)

Este es el contrato que el binario `flores` tiene que cumplir para que la suite
pueda medirlo. `tools/flores-stub` ya lo cumple; es la referencia ejecutable.

## Invocación

```sh
flores run [--prompt TEXT] [--output text|jsonl] [--base-url URL] [--model ID]
           [--api-key KEY] [--max-turns N] [--timeout DUR]
```

- Si no se pasa `--prompt`, el prompt se lee de stdin.
- Las variables `FLORES_BASE_URL`, `FLORES_MODEL` y `FLORES_API_KEY` sirven como
  valores por defecto de esos flags.
- `--output text` escribe solo la respuesta final en stdout y los diagnósticos en stderr.
- `--output jsonl` escribe en stdout **un evento JSON por línea** (la trayectoria).

| Código de salida | Significado |
|---|---|
| 0 | éxito |
| 2 | uso incorrecto (flags o prompt vacío) |
| 3 | error del proveedor (HTTP ≥ 300, red, stream roto) |
| 4 | se alcanzó `--max-turns` |

## Eventos

Esquema formal: [`schema/trajectory.v0.schema.json`](../schema/trajectory.v0.schema.json)
(JSON Schema 2020-12). Todos los eventos llevan:

| Campo | Tipo | |
|---|---|---|
| `seq` | int | empieza en 0 y sube de 1 en 1 |
| `t_ms` | number | ms desde el inicio de la sesión (reloj monotónico), nunca decrece |
| `type` | string | uno de los tipos de abajo |

| `type` | Campos obligatorios | Cuándo |
|---|---|---|
| `session.start` | `schema_version`, `agent{name,version}`, `model`, `provider{name,protocol?,base_url?}`, `cwd`, `started_at` | primer evento, una sola vez |
| `turn.start` | `turn` (≥ 1) | antes de cada llamada al modelo |
| `llm.request` | `turn`, `request_bytes`, `messages`, `tools`, `system_bytes` (+ `attempt`) | al enviar la petición |
| `llm.response` | `turn`, `status` (`ok`/`error`), `duration_ms` (+ `ttft_ms`, `http_status`, `stop_reason`, `usage`) | al terminar la respuesta |
| `message` | `turn`, `role`, `text` | texto del asistente |
| `tool.call` | `turn`, `id`, `name`, `arguments` | el modelo pide una tool |
| `tool.result` | `turn`, `id`, `ok`, `duration_ms`, `output_bytes` (+ `error`) | la tool terminó; `id` debe existir en un `tool.call` anterior |
| `context.compact` | `before_tokens`, `after_tokens` (+ `strategy`) | se compactó el historial |
| `error` | `kind` (`provider`/`tool`/`limit`/`internal`), `message`, `retryable` | algo falló |
| `session.end` | `status` (`success`/`error`/`max_turns`/`cancelled`), `turns`, `duration_ms`, `usage`, `tool_calls`, `tool_errors` (+ `final_text`) | último evento, una sola vez |

`usage` = `{input_tokens, output_tokens, cache_read_tokens?, cache_write_tokens?}`.

Ejemplo (salida real de `flores-stub` contra el escenario `tool-roundtrip`, abreviada):

```jsonl
{"seq":0,"t_ms":0.01,"type":"session.start","schema_version":"trajectory/v0","agent":{"name":"flores-stub","version":"0.0.0-stub"},"model":"mock-model",...}
{"seq":1,"t_ms":0.02,"type":"turn.start","turn":1}
{"seq":2,"t_ms":0.05,"type":"llm.request","turn":1,"request_bytes":186,"messages":2,"tools":0,"system_bytes":77,"attempt":1}
{"seq":3,"t_ms":1.2,"type":"llm.response","turn":1,"status":"ok","ttft_ms":1.1,"duration_ms":1.2,"stop_reason":"tool_calls","usage":{...}}
{"seq":4,"t_ms":1.2,"type":"tool.call","turn":1,"id":"call_1_0","name":"bash","arguments":{"command":"ls"}}
{"seq":5,"t_ms":1.2,"type":"tool.result","turn":1,"id":"call_1_0","ok":false,"duration_ms":0,"output_bytes":52,"error":"tool not implemented"}
...
{"seq":10,"t_ms":2.4,"type":"session.end","status":"success","turns":2,"duration_ms":2.4,"usage":{...},"tool_calls":1,"tool_errors":1,"final_text":"There are no files here."}
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

Los cambios incompatibles crean `trajectory/v1` en un fichero de esquema nuevo.
Añadir campos opcionales no rompe la v0.
