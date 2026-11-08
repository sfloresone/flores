# Arquitectura de flores

> 🇬🇧 [English](ARCHITECTURE.en.md)

Este documento sigue una petición de principio a fin. Las decisiones están en
los ADR: [0001](../../docs/adr/0001-estructura-y-estilo-go.md) (estructura),
[0002](../../docs/adr/0002-interfaz-provider.md) (proveedor),
[0003](../../docs/adr/0003-bucle-del-agente.md) (bucle),
[0004](../../docs/adr/0004-reintentos.md) (reintentos) y
[0005](../../docs/adr/0005-adaptador-openai.md) (adaptador OpenAI).

Estado: **hito M1** (M1a + M1b). Hay bucle, dos proveedores (Anthropic y
OpenAI Chat Completions, que también cubre los servidores compatibles) y
reintentos, pero todavía no hay tools.

## Los paquetes

| Paquete | Qué hace | Qué no hace |
|---|---|---|
| `cmd/flores` | Lee flags y variables de entorno, crea el proveedor y el agente, escribe la salida y elige el código de salida. | Lógica del agente o del protocolo. |
| `internal/agent` | El bucle: llama al modelo, responde a las tools, aplica los frenos y pide el resumen final. | Hablar HTTP, formatear la salida, reintentar. |
| `internal/provider` | Tipos propios (`Message`, `Request`, `Event`, `Error`), la interfaz `Provider`, los reintentos (`RetryPolicy`, `WithRetry`) y lo que comparten los adaptadores HTTP: el lector SSE (`ReadSSE`) y la lectura de errores (`KindForStatus`, `RetryAfter`, `ShouldRetry`). | Conocer a ningún proveedor concreto. |
| `internal/provider/anthropic` | El protocolo Messages API con `net/http`: cuerpo, cabeceras, eventos SSE y errores. | Reintentar o decidir qué hacer con las tools. |
| `internal/provider/openai` | El protocolo Chat Completions, igual: sirve para OpenAI y para cualquier servidor compatible (Ollama, vLLM, OpenRouter…) cambiando la base URL. | Lo mismo. |
| `internal/trajectory` | Traduce los eventos del agente a `trajectory/v1` (JSONL). | Decidir nada sobre la ejecución. |

## El recorrido de `flores -p "Say hello."`

1. **`main.go`** valida las opciones y crea:
   - el cliente del proveedor elegido con `--provider` (`anthropic` por
     defecto, u `openai`), con un único `http.Client` para que la conexión se
     reutilice entre pasos. La tabla `providers` dice, para cada uno, su
     adaptador, su protocolo, su base URL y su variable de clave
     (`ANTHROPIC_API_KEY` u `OPENAI_API_KEY`);
   - un `agent.Agent` con el modelo, el system prompt y `MaxSteps` (0 = sin límite).
2. **`Agent.Run(ctx, prompt)`** añade el mensaje del usuario al historial y
   empieza un **turno**. Devuelve un iterador de eventos; el último siempre es
   `EventDone`, con el resultado.
3. **Cada paso** (`turn.step`):
   - construye un `provider.Request` con todo el historial;
   - recorre `provider.Stream(...)`: `EventSent` (bytes enviados), `EventTextDelta`
     (texto según llega), `EventToolCall` (llamadas completas) y `EventDone`
     (tokens y motivo de parada);
   - añade la respuesta del asistente al historial.
4. **El adaptador** (`anthropic.Stream` u `openai.Stream`):
   - serializa el request (las tools se ordenan por nombre, para que la caché
     reciba bytes estables);
   - hace `POST /v1/messages` o `POST {base}/chat/completions`;
   - lee el stream SSE (`stream.go`, sobre `provider.ReadSSE`) y une los
     fragmentos de cada tool call (`tool_use` o `tool_calls[i]`);
   - normaliza los tokens: `InputTokens` es siempre el input no cacheado.

   Si algo falla, devuelve un `*provider.Error` ya clasificado (`Auth`,
   `Quota`, `RateLimit`, `Overloaded`…).

   Encima va **`provider.WithRetry`**. Si el error es transitorio (429, 5xx,
   stream cortado…):
   - emite `EventRetry`;
   - espera lo que diga `RetryPolicy.Delay`;
   - reenvía la misma petición.

   El paso descarta el texto parcial del intento fallido, y el historial solo
   guarda la respuesta buena. Un 401 y una cuenta sin saldo nunca se
   reintentan.
5. **Si el modelo pidió tools**, `runTools` las responde. En M0 todas devuelven
   el error `unknown tool`. Los resultados se añaden al historial y el bucle
   vuelve al paso 3.
6. **Los frenos** (`guard.go`) actúan tras cada paso con tools:
   - `--max-steps N`, si se ha puesto;
   - la detección de repeticiones: la misma llamada con el mismo resultado
     5 veces en los últimos 10 pasos.

   Si salta uno, `summarize` hace **un paso más** sin tools y pide al modelo un
   resumen de lo hecho y lo pendiente.
7. **Salida.** En modo `text`, el texto se escribe según llega y el motivo de
   parada va a stderr. En modo `jsonl`, `trajectory.Emitter` escribe un evento
   por línea.
8. **Código de salida:** 0 éxito, 2 uso, 3 proveedor o timeout, 4 freno,
   130 cancelado.

## Probar y medir

```sh
cd apps/cli && go vet ./... && go test -race ./...     # unitarios, sin red
cd benchmarks && uv run pytest -q                       # contrato contra el mock real
uv run flores-bench harness --agents flores,flores-openai   # capa B, los dos protocolos
uv run flores-bench perf hello --agents flores,fx       # capa A
```

El banco compila `flores` desde aquí en `benchmarks/.bin/flores` en cada
ejecución, así que nunca mide un binario viejo.
